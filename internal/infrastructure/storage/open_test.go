package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	appnodes "proxygateway/internal/application/nodes"
	appuow "proxygateway/internal/application/uow"
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

func TestMigrateRunsSQLiteSchema(t *testing.T) {
	t.Parallel()

	handle, err := Open(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.DB.Close() })

	if err := Migrate(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := handle.DB.QueryRow(`SELECT COUNT(*) FROM maintenance_settings WHERE id = 1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("maintenance_settings rows = %d, want 1", count)
	}
}

func TestMigrateRejectsPostgresUntilImplemented(t *testing.T) {
	t.Parallel()

	err := Migrate(context.Background(), Handle{Dialect: databaseinfra.DialectPostgres})
	if err == nil {
		t.Fatal("expected postgres migrations to be rejected until implemented")
	}
	if !strings.Contains(err.Error(), "not implemented yet") {
		t.Fatalf("postgres migration error = %q, want not implemented yet", err)
	}
}

func TestNewRepositoriesRejectsPostgresUntilImplemented(t *testing.T) {
	t.Parallel()

	_, err := NewRepositories(Handle{Dialect: databaseinfra.DialectPostgres})
	if err == nil {
		t.Fatal("expected postgres repositories to be rejected until implemented")
	}
	if !strings.Contains(err.Error(), "not implemented yet") {
		t.Fatalf("postgres repositories error = %q, want not implemented yet", err)
	}
}

func TestNewMaintenanceRunRepositoryRejectsPostgresUntilImplemented(t *testing.T) {
	t.Parallel()

	_, err := NewMaintenanceRunRepository(Handle{Dialect: databaseinfra.DialectPostgres})
	if err == nil {
		t.Fatal("expected postgres maintenance run repository to be rejected until implemented")
	}
	if !strings.Contains(err.Error(), "not implemented yet") {
		t.Fatalf("postgres maintenance run repository error = %q, want not implemented yet", err)
	}
}

func TestWithTxRejectsPostgresUntilImplemented(t *testing.T) {
	t.Parallel()

	err := (Handle{Dialect: databaseinfra.DialectPostgres}).WithTx(context.Background(), func(_ appuow.Tx) error {
		t.Fatal("transaction callback should not be called")
		return nil
	})
	if err == nil {
		t.Fatal("expected postgres transactions to be rejected until implemented")
	}
	if !strings.Contains(err.Error(), "not implemented yet") {
		t.Fatalf("postgres transaction error = %q, want not implemented yet", err)
	}
}

func TestWithTxCommitsSQLiteTransaction(t *testing.T) {
	t.Parallel()

	handle, err := Open(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.DB.Close() })
	if err := Migrate(context.Background(), handle); err != nil {
		t.Fatal(err)
	}

	service := appnodes.Service{
		NewNodeID: func() (string, error) {
			return "node_commit", nil
		},
	}
	err = handle.WithTx(context.Background(), func(tx appuow.Tx) error {
		_, err := service.Upsert(tx.NodeUpsertRepository(), appnodes.UpsertInput{
			Fingerprint:  "fp_commit",
			Name:         "Commit Node",
			Type:         "http",
			Server:       "127.0.0.1",
			ServerPort:   18080,
			OutboundJSON: `{"type":"http","server":"127.0.0.1","server_port":18080}`,
			SourceID:     "manual",
			SourceName:   "Manual",
			SourceType:   "manual",
			NowMillis:    1000,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	repos, err := NewRepositories(handle)
	if err != nil {
		t.Fatal(err)
	}
	node, found, err := repos.Node.Load(context.Background(), "node_commit")
	if err != nil {
		t.Fatal(err)
	}
	if !found || node.Name != "Commit Node" {
		t.Fatalf("committed node = %#v found=%t", node, found)
	}
}

func TestWithTxRollsBackSQLiteTransactionOnError(t *testing.T) {
	t.Parallel()

	handle, err := Open(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.DB.Close() })
	if err := Migrate(context.Background(), handle); err != nil {
		t.Fatal(err)
	}

	rollbackErr := errors.New("rollback please")
	service := appnodes.Service{
		NewNodeID: func() (string, error) {
			return "node_rollback", nil
		},
	}
	err = handle.WithTx(context.Background(), func(tx appuow.Tx) error {
		if _, err := service.Upsert(tx.NodeUpsertRepository(), appnodes.UpsertInput{
			Fingerprint:  "fp_rollback",
			Name:         "Rollback Node",
			Type:         "http",
			Server:       "127.0.0.1",
			ServerPort:   18081,
			OutboundJSON: `{"type":"http","server":"127.0.0.1","server_port":18081}`,
			SourceID:     "manual",
			SourceName:   "Manual",
			SourceType:   "manual",
			NowMillis:    1000,
		}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("WithTx error = %v, want rollbackErr", err)
	}

	repos, err := NewRepositories(handle)
	if err != nil {
		t.Fatal(err)
	}
	_, found, err := repos.Node.Load(context.Background(), "node_rollback")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("node_rollback should have been rolled back")
	}
}
