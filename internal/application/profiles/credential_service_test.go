package profiles

import (
	"context"
	"errors"
	"testing"
)

func TestCredentialServiceCreateStoresCredentialAndReturnsView(t *testing.T) {
	repo := &fakeCredentialRepository{
		profileExists: true,
		identifier:    "work",
	}
	service := NewCredentialService(repo, func() (string, error) {
		return "cred_1", nil
	}, func() int64 {
		return 1234
	})

	credential, err := service.Create(context.Background(), CreateCredentialCommand{
		ProfileID: "profile_1",
		Remark:    " client ",
		Password:  "secret1",
		Endpoint:  "127.0.0.1:28080",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !repo.createCalled {
		t.Fatal("CreateCredential was not called")
	}
	if repo.created.ID != "cred_1" || repo.created.ProfileID != "profile_1" || repo.created.Remark != "client" || repo.created.CreatedAt != 1234 {
		t.Fatalf("created record = %#v", repo.created)
	}
	if credential.ID != "cred_1" || credential.HTTPProxyURL != "http://work:secret1@127.0.0.1:28080" {
		t.Fatalf("credential view = %#v", credential)
	}
}

func TestCredentialServiceCreateRejectsDuplicatePassword(t *testing.T) {
	repo := &fakeCredentialRepository{profileExists: true, passwordExists: true}
	service := NewCredentialService(repo, func() (string, error) {
		return "cred_1", nil
	}, func() int64 {
		return 1234
	})

	_, err := service.Create(context.Background(), CreateCredentialCommand{
		ProfileID: "profile_1",
		Remark:    "client",
		Password:  "secret1",
		Endpoint:  "127.0.0.1:28080",
	})
	if !errors.Is(err, ErrDuplicateCredential) {
		t.Fatalf("err = %v, want ErrDuplicateCredential", err)
	}
	if repo.createCalled {
		t.Fatal("CreateCredential should not be called for duplicate password")
	}
}

func TestCredentialServiceListRequiresExistingProfile(t *testing.T) {
	service := NewCredentialService(&fakeCredentialRepository{}, nil, nil)

	_, err := service.List(context.Background(), "profile_missing", "127.0.0.1:28080")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("err = %v, want ErrProfileNotFound", err)
	}
}

func TestCredentialServiceSetEnabledRequiresExistingCredential(t *testing.T) {
	repo := &fakeCredentialRepository{}
	service := NewCredentialService(repo, nil, nil)

	_, err := service.SetEnabled(context.Background(), "profile_1", "cred_missing", true)
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("err = %v, want ErrCredentialNotFound", err)
	}
}

type fakeCredentialRepository struct {
	profileExists  bool
	identifier     string
	passwordExists bool
	records        []CredentialRecord
	created        CredentialCreateRecord
	createCalled   bool
	setUpdated     bool
	deleteDeleted  bool
}

func (r *fakeCredentialRepository) ProfileExists(context.Context, string) (bool, error) {
	return r.profileExists, nil
}

func (r *fakeCredentialRepository) LoadProfileIdentifier(_ context.Context, profileID string) (string, bool, error) {
	if r.identifier == "" {
		return profileID, false, nil
	}
	return r.identifier, true, nil
}

func (r *fakeCredentialRepository) PasswordExists(context.Context, string, string) (bool, error) {
	return r.passwordExists, nil
}

func (r *fakeCredentialRepository) CreateCredential(_ context.Context, record CredentialCreateRecord) error {
	r.createCalled = true
	r.created = record
	return nil
}

func (r *fakeCredentialRepository) ListCredentials(context.Context, string) ([]CredentialRecord, error) {
	return r.records, nil
}

func (r *fakeCredentialRepository) SetCredentialEnabled(context.Context, string, string, bool) (bool, error) {
	return r.setUpdated, nil
}

func (r *fakeCredentialRepository) DeleteCredential(context.Context, string, string) (bool, error) {
	return r.deleteDeleted, nil
}

func (r *fakeCredentialRepository) CountCredentials(context.Context, string) (CredentialCounts, error) {
	return CredentialCounts{}, nil
}
