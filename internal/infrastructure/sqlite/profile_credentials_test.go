package sqlite

import (
	"context"
	"testing"

	appprofiles "proxygateway/internal/application/profiles"
)

func TestProfileCredentialRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := NewProfileCredentialRepository(db)

	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, created_at) VALUES ('profile_1', 'work', 'Work', 'fixed', 100)`)

	exists, err := repo.ProfileExists(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected profile_1 to exist")
	}
	identifier, found, err := repo.LoadProfileIdentifier(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || identifier != "work" {
		t.Fatalf("identifier = %q found=%t", identifier, found)
	}

	if err := repo.CreateCredential(ctx, appprofiles.CredentialCreateRecord{
		ID:        "cred_1",
		ProfileID: "profile_1",
		Remark:    "client",
		Password:  "secret",
		CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	duplicate, err := repo.PasswordExists(ctx, "profile_1", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate {
		t.Fatal("expected duplicate password")
	}
	records, err := repo.ListCredentials(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != "cred_1" || records[0].Remark != "client" || !records[0].Enabled {
		t.Fatalf("records = %#v", records)
	}
	counts, err := repo.CountCredentials(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 || counts.Enabled != 1 {
		t.Fatalf("counts = %#v", counts)
	}

	updated, err := repo.SetCredentialEnabled(ctx, "profile_1", "cred_1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("expected credential update")
	}
	counts, err = repo.CountCredentials(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 || counts.Enabled != 0 {
		t.Fatalf("counts after disable = %#v", counts)
	}

	deleted, err := repo.DeleteCredential(ctx, "profile_1", "cred_1")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected credential delete")
	}
	deleted, err = repo.DeleteCredential(ctx, "profile_1", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("missing credential should not delete")
	}
}
