package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	PasswordMinLength = 12
	SessionTTL        = 30 * 24 * time.Hour
)

var (
	ErrCredentialExists         = errors.New("admin credential already exists")
	ErrPasswordTooShort         = errors.New("admin password too short")
	ErrInvalidCredentials       = errors.New("invalid credentials")
	ErrCurrentPasswordIncorrect = errors.New("current password is incorrect")
)

type Service struct {
	Repo Repository
	Now  func() int64
}

func (s Service) RequiresSetup(ctx context.Context) (bool, error) {
	exists, err := s.Repo.HasCredential(ctx)
	if err != nil {
		return false, err
	}
	return !exists, nil
}

func (s Service) Setup(ctx context.Context, password string) (string, error) {
	if len(password) < PasswordMinLength {
		return "", ErrPasswordTooShort
	}
	exists, err := s.Repo.HasCredential(ctx)
	if err != nil {
		return "", err
	}
	if exists {
		return "", ErrCredentialExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if err := s.Repo.CreateCredential(ctx, CredentialRecord{
		PasswordHash: string(hash),
		CreatedAt:    s.nowMillis(),
	}); err != nil {
		return "", err
	}
	return s.createSession(ctx)
}

func (s Service) Login(ctx context.Context, password string) (string, error) {
	passwordHash, ok, err := s.Repo.LoadPasswordHash(ctx)
	if err != nil {
		return "", err
	}
	if !ok || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return "", ErrInvalidCredentials
	}
	return s.createSession(ctx)
}

func (s Service) ChangePassword(ctx context.Context, currentPassword, newPassword string) error {
	if len(newPassword) < PasswordMinLength {
		return ErrPasswordTooShort
	}
	hash, ok, err := s.Repo.LoadPasswordHash(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(currentPassword)); err != nil {
		return ErrCurrentPasswordIncorrect
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.Repo.UpdatePasswordAndDeleteSessions(ctx, string(newHash))
}

func (s Service) Authenticated(ctx context.Context, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	hash := tokenHash(token)
	createdAt, ok, err := s.Repo.LoadSessionCreatedAt(ctx, hash)
	if err != nil || !ok {
		return false
	}
	if sessionExpired(createdAt, s.nowMillis()) {
		_ = s.Repo.DeleteSession(ctx, hash)
		return false
	}
	return true
}

func (s Service) createSession(ctx context.Context) (string, error) {
	now := s.nowMillis()
	s.deleteExpiredSessions(ctx, now)
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := s.Repo.CreateSession(ctx, SessionRecord{
		TokenHash: tokenHash(token),
		CreatedAt: now,
	}); err != nil {
		return "", err
	}
	return token, nil
}

func (s Service) deleteExpiredSessions(ctx context.Context, now int64) {
	cutoff := now - int64(SessionTTL/time.Millisecond)
	_ = s.Repo.DeleteExpiredSessions(ctx, cutoff)
}

func (s Service) nowMillis() int64 {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UnixMilli()
}

func sessionExpired(createdAt, now int64) bool {
	if createdAt <= 0 {
		return true
	}
	return now-createdAt > int64(SessionTTL/time.Millisecond)
}

func randomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
