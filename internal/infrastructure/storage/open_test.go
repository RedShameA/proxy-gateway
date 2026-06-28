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

func TestConfigFromEnvDefaultsToSQLite(t *testing.T) {
	t.Parallel()

	config, err := ConfigFromEnv("/data", func(string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}

	if config.Driver != DriverSQLite {
		t.Fatalf("Driver = %q, want %q", config.Driver, DriverSQLite)
	}
	if config.DataDir != "/data" {
		t.Fatalf("DataDir = %q, want /data", config.DataDir)
	}
	if config.DSN != "" {
		t.Fatalf("DSN = %q, want empty", config.DSN)
	}
}

func TestConfigFromEnvNormalizesPostgresAlias(t *testing.T) {
	t.Parallel()

	lookup := func(key string) (string, bool) {
		switch key {
		case EnvDBDriver:
			return " postgresql ", true
		case EnvDBDSN:
			return "postgres://proxy:secret@example.local/proxygateway", true
		default:
			return "", false
		}
	}
	config, err := ConfigFromEnv("/data", lookup)
	if err != nil {
		t.Fatal(err)
	}

	if config.Driver != DriverPostgres {
		t.Fatalf("Driver = %q, want %q", config.Driver, DriverPostgres)
	}
	if config.DSN != "postgres://proxy:secret@example.local/proxygateway" {
		t.Fatalf("DSN was not preserved in config")
	}
}

func TestConfigFromEnvRejectsDSNWithoutDriver(t *testing.T) {
	t.Parallel()

	secretDSN := "postgres://proxy:super-secret@example.local/proxygateway"
	_, err := ConfigFromEnv("/data", func(key string) (string, bool) {
		if key == EnvDBDSN {
			return secretDSN, true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("expected DSN-only config to be rejected")
	}
	if strings.Contains(err.Error(), secretDSN) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("error leaked DSN secret: %v", err)
	}
	if !strings.Contains(err.Error(), EnvDBDriver) {
		t.Fatalf("error = %q, want mention %s", err, EnvDBDriver)
	}
}

func TestConfigFromEnvRejectsInvalidDriverWithoutLeakingDSN(t *testing.T) {
	t.Parallel()

	secretDSN := "postgres://proxy:super-secret@example.local/proxygateway"
	_, err := ConfigFromEnv("/data", func(key string) (string, bool) {
		switch key {
		case EnvDBDriver:
			return "mysql", true
		case EnvDBDSN:
			return secretDSN, true
		default:
			return "", false
		}
	})
	if err == nil {
		t.Fatal("expected invalid driver to be rejected")
	}
	if strings.Contains(err.Error(), secretDSN) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("error leaked DSN secret: %v", err)
	}
	if !strings.Contains(err.Error(), "mysql") {
		t.Fatalf("error = %q, want invalid driver value", err)
	}
}

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

func TestOpenRejectsPostgresWithoutDSN(t *testing.T) {
	t.Parallel()

	_, err := Open(Config{Driver: string(databaseinfra.DialectPostgres)})
	if err == nil {
		t.Fatal("expected postgres driver without DSN to be rejected")
	}
	if !strings.Contains(err.Error(), "DSN is empty") {
		t.Fatalf("postgres error = %q, want empty DSN", err)
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

func TestMigrateRejectsNilPostgresHandle(t *testing.T) {
	t.Parallel()

	err := Migrate(context.Background(), Handle{Dialect: databaseinfra.DialectPostgres})
	if err == nil {
		t.Fatal("expected nil postgres handle to be rejected")
	}
	if !strings.Contains(err.Error(), "database handle is nil") {
		t.Fatalf("postgres migration error = %q, want nil handle", err)
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
