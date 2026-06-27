package sqlite

import (
	"context"
	"testing"

	appadmin "proxygateway/internal/application/admin"
)

func TestAdminRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	repo := NewAdminRepository(db)
	ctx := context.Background()

	if exists, err := repo.HasCredential(ctx); err != nil || exists {
		t.Fatalf("fresh HasCredential = %t, %v", exists, err)
	}
	if _, ok, err := repo.LoadPasswordHash(ctx); err != nil || ok {
		t.Fatalf("fresh LoadPasswordHash ok=%t err=%v", ok, err)
	}
	if err := repo.CreateCredential(ctx, appadmin.CredentialRecord{PasswordHash: "hash1", CreatedAt: 1000}); err != nil {
		t.Fatal(err)
	}
	if exists, err := repo.HasCredential(ctx); err != nil || !exists {
		t.Fatalf("HasCredential = %t, %v", exists, err)
	}
	if hash, ok, err := repo.LoadPasswordHash(ctx); err != nil || !ok || hash != "hash1" {
		t.Fatalf("LoadPasswordHash = %q ok=%t err=%v", hash, ok, err)
	}

	if err := repo.CreateSession(ctx, appadmin.SessionRecord{TokenHash: "token_old", CreatedAt: 1000}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateSession(ctx, appadmin.SessionRecord{TokenHash: "token_new", CreatedAt: 3000}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteExpiredSessions(ctx, 1500); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repo.LoadSessionCreatedAt(ctx, "token_old"); err != nil || ok {
		t.Fatalf("old session ok=%t err=%v", ok, err)
	}
	if createdAt, ok, err := repo.LoadSessionCreatedAt(ctx, "token_new"); err != nil || !ok || createdAt != 3000 {
		t.Fatalf("new session createdAt=%d ok=%t err=%v", createdAt, ok, err)
	}

	if err := repo.UpdatePasswordAndDeleteSessions(ctx, "hash2"); err != nil {
		t.Fatal(err)
	}
	if hash, ok, err := repo.LoadPasswordHash(ctx); err != nil || !ok || hash != "hash2" {
		t.Fatalf("updated hash = %q ok=%t err=%v", hash, ok, err)
	}
	if _, ok, err := repo.LoadSessionCreatedAt(ctx, "token_new"); err != nil || ok {
		t.Fatalf("session after password update ok=%t err=%v", ok, err)
	}
}
