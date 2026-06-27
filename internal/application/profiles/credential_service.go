package profiles

import (
	"context"
	"errors"
	"strings"

	domainprofile "proxygateway/internal/domain/profile"
)

var (
	ErrDuplicateCredential = errors.New("duplicate proxy credential")
	ErrCredentialNotFound  = errors.New("proxy credential not found")
)

type CredentialIDGenerator func() (string, error)

type CredentialClock func() int64

type CredentialService struct {
	repo  CredentialRepository
	newID CredentialIDGenerator
	now   CredentialClock
}

type CreateCredentialCommand struct {
	ProfileID string
	Remark    string
	Password  string
	Endpoint  string
}

type UpdateCredentialResult struct {
	Updated bool `json:"updated"`
}

type DeleteCredentialResult struct {
	Deleted bool `json:"deleted"`
}

func NewCredentialService(repo CredentialRepository, newID CredentialIDGenerator, now CredentialClock) CredentialService {
	return CredentialService{
		repo:  repo,
		newID: newID,
		now:   now,
	}
}

func (s CredentialService) Create(ctx context.Context, cmd CreateCredentialCommand) (CreatedCredential, error) {
	if err := domainprofile.ValidateProxyCredential(cmd.Remark, cmd.Password); err != nil {
		return CreatedCredential{}, err
	}
	exists, err := s.repo.ProfileExists(ctx, cmd.ProfileID)
	if err != nil {
		return CreatedCredential{}, err
	}
	if !exists {
		return CreatedCredential{}, ErrProfileNotFound
	}
	duplicate, err := s.repo.PasswordExists(ctx, cmd.ProfileID, cmd.Password)
	if err != nil {
		return CreatedCredential{}, err
	}
	if duplicate {
		return CreatedCredential{}, ErrDuplicateCredential
	}
	id, err := s.newID()
	if err != nil {
		return CreatedCredential{}, err
	}
	remark := strings.TrimSpace(cmd.Remark)
	if err := s.repo.CreateCredential(ctx, CredentialCreateRecord{
		ID:        id,
		ProfileID: cmd.ProfileID,
		Remark:    remark,
		Password:  cmd.Password,
		CreatedAt: s.nowMillis(),
	}); err != nil {
		return CreatedCredential{}, err
	}
	return BuildCreatedCredential(CreatedCredentialInput{
		ID:                id,
		ProfileID:         cmd.ProfileID,
		Remark:            remark,
		Password:          cmd.Password,
		ProfileIdentifier: s.profileIdentifier(ctx, cmd.ProfileID),
		Endpoint:          cmd.Endpoint,
	}), nil
}

func (s CredentialService) List(ctx context.Context, profileID, endpoint string) (CredentialList, error) {
	exists, err := s.repo.ProfileExists(ctx, profileID)
	if err != nil {
		return CredentialList{}, err
	}
	if !exists {
		return CredentialList{}, ErrProfileNotFound
	}
	records, err := s.repo.ListCredentials(ctx, profileID)
	if err != nil {
		return CredentialList{}, err
	}
	return BuildCredentialList(records, s.profileIdentifier(ctx, profileID), endpoint), nil
}

func (s CredentialService) SetEnabled(ctx context.Context, profileID, credentialID string, enabled bool) (UpdateCredentialResult, error) {
	updated, err := s.repo.SetCredentialEnabled(ctx, profileID, credentialID, enabled)
	if err != nil {
		return UpdateCredentialResult{}, err
	}
	if !updated {
		return UpdateCredentialResult{}, ErrCredentialNotFound
	}
	return UpdateCredentialResult{Updated: true}, nil
}

func (s CredentialService) Delete(ctx context.Context, profileID, credentialID string) (DeleteCredentialResult, error) {
	exists, err := s.repo.ProfileExists(ctx, profileID)
	if err != nil {
		return DeleteCredentialResult{}, err
	}
	if !exists {
		return DeleteCredentialResult{}, ErrProfileNotFound
	}
	deleted, err := s.repo.DeleteCredential(ctx, profileID, credentialID)
	if err != nil {
		return DeleteCredentialResult{}, err
	}
	if !deleted {
		return DeleteCredentialResult{}, ErrCredentialNotFound
	}
	return DeleteCredentialResult{Deleted: true}, nil
}

func (s CredentialService) profileIdentifier(ctx context.Context, profileID string) string {
	profileIdentifier, found, err := s.repo.LoadProfileIdentifier(ctx, profileID)
	if err != nil || !found || profileIdentifier == "" {
		return profileID
	}
	return profileIdentifier
}

func (s CredentialService) nowMillis() int64 {
	if s.now == nil {
		return 0
	}
	return s.now()
}
