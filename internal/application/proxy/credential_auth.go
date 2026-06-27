package proxy

import (
	"context"
	"strings"
)

type CredentialAuthResult struct {
	Credential CredentialRecord
	Failure    Failure
	LookupErr  error
	TouchErr   error
	OK         bool
}

func AuthenticateCredential(ctx context.Context, repo CredentialRepository, profileIdentifier, password string, nowMillis, touchWindowMillis int64) CredentialAuthResult {
	profileIdentifier = strings.TrimSpace(profileIdentifier)
	record, profileFound, credentialFound, err := repo.LookupCredential(ctx, profileIdentifier, password)
	if err != nil {
		return CredentialAuthResult{Failure: InvalidProxyCredentialsFailure(), LookupErr: err}
	}
	if !profileFound {
		return CredentialAuthResult{Failure: AccessProfileNotFoundFailure()}
	}
	if !credentialFound {
		return CredentialAuthResult{Failure: InvalidProxyCredentialsFailure()}
	}
	result := CredentialAuthResult{Credential: record, OK: true}
	if touchWindowMillis <= 0 {
		touchWindowMillis = 60_000
	}
	if err := repo.TouchCredentialLastUsed(ctx, record.ID, nowMillis, nowMillis-touchWindowMillis); err != nil {
		result.TouchErr = err
	}
	return result
}
