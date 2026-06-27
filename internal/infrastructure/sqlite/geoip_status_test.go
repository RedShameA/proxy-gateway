package sqlite

import (
	"context"
	"testing"

	appgeoip "proxygateway/internal/application/geoip"
)

func TestGeoIPStatusRepositoryStoresAndPreservesNonEmptyFields(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	repo := NewGeoIPStatusRepository(db)
	ctx := context.Background()

	if err := repo.StoreStatus(ctx, appgeoip.StatusUpdate{
		FilePath:  "/data/country.mmdb",
		LoadedAt:  1000,
		UpdatedAt: 2000,
		SHA256:    "abc",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.StoreStatus(ctx, appgeoip.StatusUpdate{LastError: "download failed"}); err != nil {
		t.Fatal(err)
	}
	status, err := repo.LoadStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.FilePath != "/data/country.mmdb" || status.LoadedAt != 1000 || status.UpdatedAt != 2000 || status.SHA256 != "abc" {
		t.Fatalf("status preserved fields = %#v", status)
	}
	if status.LastError != "download failed" {
		t.Fatalf("LastError = %q", status.LastError)
	}
}
