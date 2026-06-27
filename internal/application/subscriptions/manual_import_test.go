package subscriptions

import (
	"errors"
	"testing"
)

func TestParseManualNodeImportRequiresContent(t *testing.T) {
	_, _, err := ParseManualNodeImport("  ")
	if !errors.Is(err, ErrManualImportRequired) {
		t.Fatalf("ParseManualNodeImport error = %v, want ErrManualImportRequired", err)
	}
}

func TestParseManualNodeImportParsesSingleSingBoxOutbound(t *testing.T) {
	nodes, skipped, err := ParseManualNodeImport(`{"type":"http","server":"127.0.0.1","server_port":18080}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Type != "http" || nodes[0].Server != "127.0.0.1" {
		t.Fatalf("nodes = %#v", nodes)
	}
	if skipped.Count() != 0 {
		t.Fatalf("skipped = %#v", skipped.Rows())
	}
}

func TestParseManualNodeImportFallsBackToSubscriptionParser(t *testing.T) {
	nodes, skipped, err := ParseManualNodeImport("http://user:pass@127.0.0.1:18080#manual")
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].Type != "http" || nodes[0].Server != "127.0.0.1" {
		t.Fatalf("nodes = %#v", nodes)
	}
	if skipped.Count() != 0 {
		t.Fatalf("skipped = %#v", skipped.Rows())
	}
}
