package subscriptions

import (
	"context"
	"errors"
	"testing"
)

func TestManualNodeImportServiceParsesAndUpsertsManualNodes(t *testing.T) {
	tx := &fakeImportTx{}
	runner := &fakeManualNodeImportTxRunner{tx: tx}
	service := ManualNodeImportService{
		Runner: runner,
		NewNodeID: func() (string, error) {
			return "node_manual", nil
		},
	}

	result, err := service.Import(context.Background(), ManualNodeImportCommand{
		ImportText: `{"type":"http","tag":"Manual HTTP","server":"127.0.0.1","server_port":8080}`,
		NowMillis:  123,
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if !runner.called {
		t.Fatal("expected manual node import to run in transaction")
	}
	if len(result.Nodes) != 1 || result.Nodes[0].ID != "node_manual" || result.Nodes[0].Name != "Manual HTTP" {
		t.Fatalf("result nodes = %#v", result.Nodes)
	}
	if len(tx.upserts.created) != 1 || tx.upserts.created[0].ID != "node_manual" {
		t.Fatalf("created nodes = %#v", tx.upserts.created)
	}
	if len(tx.upserts.bound) != 1 || tx.upserts.bound[0].SourceID != "manual" || tx.upserts.bound[0].SourceType != "manual" {
		t.Fatalf("bound sources = %#v", tx.upserts.bound)
	}
}

func TestManualNodeImportServiceRejectsEmptyInput(t *testing.T) {
	service := ManualNodeImportService{
		Runner: &fakeManualNodeImportTxRunner{tx: &fakeImportTx{}},
	}

	_, err := service.Import(context.Background(), ManualNodeImportCommand{})
	if !errors.Is(err, ErrManualImportRequired) {
		t.Fatalf("Import() error = %v, want ErrManualImportRequired", err)
	}
}

type fakeManualNodeImportTxRunner struct {
	tx     *fakeImportTx
	called bool
}

func (r *fakeManualNodeImportTxRunner) WithManualNodeImportTx(_ context.Context, fn func(ManualNodeImportTx) error) error {
	r.called = true
	return fn(r.tx)
}
