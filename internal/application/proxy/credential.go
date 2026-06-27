package proxy

import "context"

type CredentialRecord struct {
	ID        string
	Remark    string
	ProfileID string
}

type CredentialRepository interface {
	LookupCredential(ctx context.Context, profileIdentifier, password string) (record CredentialRecord, profileFound bool, credentialFound bool, err error)
	TouchCredentialLastUsed(ctx context.Context, credentialID string, nowMillis, cutoffMillis int64) error
}
