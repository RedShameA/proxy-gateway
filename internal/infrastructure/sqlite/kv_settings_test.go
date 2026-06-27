package sqlite

import (
	"context"
	"testing"
)

func TestKVSettingsRepositoryGetSet(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	repo := NewKVSettingsRepository(db)
	ctx := context.Background()

	if value, ok, err := repo.Get(ctx, "missing"); err != nil || ok || value != "" {
		t.Fatalf("missing get = value %q ok %t err %v", value, ok, err)
	}
	if err := repo.Set(ctx, "public_proxy_endpoint", "127.0.0.1:8080"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, "public_proxy_endpoint", "127.0.0.1:9090"); err != nil {
		t.Fatal(err)
	}
	value, ok, err := repo.Get(ctx, "public_proxy_endpoint")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "127.0.0.1:9090" {
		t.Fatalf("get = value %q ok %t", value, ok)
	}
}
