package proxy

import (
	"context"
	"errors"
	"testing"
)

func TestAuthenticateCredentialReturnsCredentialAndTouchesLastUsed(t *testing.T) {
	repo := &fakeCredentialAuthRepository{
		record:          CredentialRecord{ID: "cred_1", Remark: "client", ProfileID: "profile_1"},
		profileFound:    true,
		credentialFound: true,
	}

	result := AuthenticateCredential(context.Background(), repo, " client-a ", "secret", 1000, 60_000)

	if !result.OK || result.Credential.ID != "cred_1" || result.Failure != (Failure{}) {
		t.Fatalf("result = %#v", result)
	}
	if repo.lookupProfileIdentifier != "client-a" || repo.lookupPassword != "secret" {
		t.Fatalf("lookup = %q/%q", repo.lookupProfileIdentifier, repo.lookupPassword)
	}
	if repo.touchCredentialID != "cred_1" || repo.touchNow != 1000 || repo.touchCutoff != -59000 {
		t.Fatalf("touch = %#v", repo)
	}
}

func TestAuthenticateCredentialClassifiesLookupOutcomes(t *testing.T) {
	tests := []struct {
		name string
		repo *fakeCredentialAuthRepository
		want Failure
	}{
		{
			name: "lookup error",
			repo: &fakeCredentialAuthRepository{lookupErr: errors.New("db down")},
			want: InvalidProxyCredentialsFailure(),
		},
		{
			name: "profile missing",
			repo: &fakeCredentialAuthRepository{},
			want: AccessProfileNotFoundFailure(),
		},
		{
			name: "credential missing",
			repo: &fakeCredentialAuthRepository{profileFound: true},
			want: InvalidProxyCredentialsFailure(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AuthenticateCredential(context.Background(), tt.repo, "client-a", "secret", 1000, 60_000)
			if result.OK || result.Failure != tt.want {
				t.Fatalf("result = %#v, want failure %#v", result, tt.want)
			}
		})
	}
}

type fakeCredentialAuthRepository struct {
	record                  CredentialRecord
	profileFound            bool
	credentialFound         bool
	lookupErr               error
	touchErr                error
	lookupProfileIdentifier string
	lookupPassword          string
	touchCredentialID       string
	touchNow                int64
	touchCutoff             int64
}

func (f *fakeCredentialAuthRepository) LookupCredential(_ context.Context, profileIdentifier, password string) (CredentialRecord, bool, bool, error) {
	f.lookupProfileIdentifier = profileIdentifier
	f.lookupPassword = password
	return f.record, f.profileFound, f.credentialFound, f.lookupErr
}

func (f *fakeCredentialAuthRepository) TouchCredentialLastUsed(_ context.Context, credentialID string, nowMillis, cutoffMillis int64) error {
	f.touchCredentialID = credentialID
	f.touchNow = nowMillis
	f.touchCutoff = cutoffMillis
	return f.touchErr
}
