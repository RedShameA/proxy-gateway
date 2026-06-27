package profiles

import "context"

type CredentialRecord struct {
	ID         string
	ProfileID  string
	Remark     string
	Password   string
	Enabled    bool
	CreatedAt  int64
	LastUsedAt int64
}

type CredentialCreateRecord struct {
	ID        string
	ProfileID string
	Remark    string
	Password  string
	CreatedAt int64
}

type CredentialCounts struct {
	Total   int
	Enabled int
}

type CredentialRepository interface {
	ProfileExists(ctx context.Context, profileID string) (bool, error)
	LoadProfileIdentifier(ctx context.Context, profileID string) (string, bool, error)
	PasswordExists(ctx context.Context, profileID, password string) (bool, error)
	CreateCredential(ctx context.Context, record CredentialCreateRecord) error
	ListCredentials(ctx context.Context, profileID string) ([]CredentialRecord, error)
	SetCredentialEnabled(ctx context.Context, profileID, credentialID string, enabled bool) (bool, error)
	DeleteCredential(ctx context.Context, profileID, credentialID string) (bool, error)
	CountCredentials(ctx context.Context, profileID string) (CredentialCounts, error)
}
