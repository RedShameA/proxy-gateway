package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
	storageinfra "proxygateway/internal/infrastructure/storage"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func NewForTest(t testing.TB, opts ...Option) *Gateway {
	t.Helper()
	g, err := New(t.TempDir(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func TestNewUsesInjectedStorageConfig(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	sqlitePath := filepath.Join(t.TempDir(), "custom.sqlite")
	g, err := New(dataDir, WithStorageConfig(storageinfra.Config{
		Driver:     storageinfra.DriverSQLite,
		SQLitePath: sqlitePath,
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })

	if _, err := os.Stat(sqlitePath); err != nil {
		t.Fatalf("custom sqlite path was not used: %v", err)
	}
	defaultPath := sqliteinfra.DefaultPath(dataDir)
	if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
		t.Fatalf("default sqlite path exists or stat failed unexpectedly: %v", err)
	}
}

func TestNewDoesNotLogStorageDSN(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zapcore.DebugLevel)
	secretDSN := "postgres://proxy:super-secret@example.local/proxygateway"
	_, err := New(t.TempDir(),
		WithLogger(zap.New(core)),
		WithStorageConfig(storageinfra.Config{
			Driver: "mysql",
			DSN:    secretDSN,
		}),
	)
	if err == nil {
		t.Fatal("expected invalid storage driver to fail")
	}

	for _, entry := range observed.All() {
		if strings.Contains(entry.Message, secretDSN) || strings.Contains(entry.Message, "super-secret") {
			t.Fatalf("log message leaked DSN secret: %q", entry.Message)
		}
		for _, field := range entry.Context {
			value := field.String
			if strings.Contains(value, secretDSN) || strings.Contains(value, "super-secret") {
				t.Fatalf("log field %q leaked DSN secret: %q", field.Key, value)
			}
		}
	}
}
