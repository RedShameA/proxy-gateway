package admin

import "context"

type CredentialRecord struct {
	PasswordHash string
	CreatedAt    int64
}

type SessionRecord struct {
	TokenHash string
	CreatedAt int64
}

type Repository interface {
	HasCredential(ctx context.Context) (bool, error)
	CreateCredential(ctx context.Context, record CredentialRecord) error
	LoadPasswordHash(ctx context.Context) (string, bool, error)
	CreateSession(ctx context.Context, record SessionRecord) error
	LoadSessionCreatedAt(ctx context.Context, tokenHash string) (int64, bool, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteExpiredSessions(ctx context.Context, cutoffMillis int64) error
	UpdatePasswordAndDeleteSessions(ctx context.Context, passwordHash string) error
}
