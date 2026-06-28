package postgres

import (
	"os"
	"strings"
	"testing"
)

func TestNodeCreateInsertUsesPostgresReturning(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("node_transactions.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "LastInsertId") {
		t.Fatal("postgres node insert must not use LastInsertId")
	}
	if !strings.Contains(text, "INSERT INTO nodes") || !strings.Contains(text, "RETURNING id") {
		t.Fatal("postgres node insert should use RETURNING id")
	}
}
