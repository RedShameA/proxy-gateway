package postgres

import (
	"strings"
	"testing"
)

func TestOpenConnectionErrorDoesNotLeakDSN(t *testing.T) {
	t.Parallel()

	secretDSN := "postgres://proxy:super-secret@127.0.0.1:1/proxygateway?connect_timeout=1"
	db, err := Open(secretDSN)
	if err == nil {
		_ = db.Close()
		t.Fatal("expected postgres connection to fail")
	}
	if strings.Contains(err.Error(), secretDSN) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("postgres open error leaked DSN secret: %v", err)
	}
}
