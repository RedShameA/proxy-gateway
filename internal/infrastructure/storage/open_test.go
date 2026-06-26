package storage

import (
	"strings"
	"testing"

	databaseinfra "proxygateway/internal/infrastructure/database"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
)

func TestOpenDefaultsToSQLiteDataDirPath(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	handle, err := Open(Config{DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.DB.Close() })

	if handle.Dialect != databaseinfra.DialectSQLite {
		t.Fatalf("dialect = %q, want %q", handle.Dialect, databaseinfra.DialectSQLite)
	}
	var journalMode string
	if err := handle.DB.QueryRow(`PRAGMA journal_mode = WAL`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestOpenSQLiteUsesExplicitPath(t *testing.T) {
	t.Parallel()

	path := sqliteinfra.DefaultPath(t.TempDir())
	handle, err := Open(Config{Driver: DriverSQLite, SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.DB.Close() })

	if _, err := handle.DB.Exec(`CREATE TABLE explicit_path_probe (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRejectsPostgresUntilImplemented(t *testing.T) {
	t.Parallel()

	_, err := Open(Config{Driver: string(databaseinfra.DialectPostgres), DSN: "postgres://example"})
	if err == nil {
		t.Fatal("expected postgres driver to be rejected until implemented")
	}
	if !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("postgres error = %q, want not supported yet", err)
	}
}
