package nodes

import (
	"context"
	"testing"
)

func TestDeleteServiceRunsManualSourceDeleteInTransaction(t *testing.T) {
	repo := &fakeDeleteRepo{
		affected:            1,
		deletedFingerprints: []string{"fp-1"},
	}
	runner := &fakeNodeDeleteTxRunner{repo: repo}
	service := DeleteService{Runner: runner}

	result, err := service.DeleteManualSource(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("DeleteManualSource() error = %v", err)
	}
	if !runner.called {
		t.Fatal("expected delete service to use transaction runner")
	}
	if repo.deletedNodeID != "node-1" {
		t.Fatalf("deleted node id = %q", repo.deletedNodeID)
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp-1" {
		t.Fatalf("result = %#v", result)
	}
}

type fakeNodeDeleteTxRunner struct {
	repo   *fakeDeleteRepo
	called bool
}

func (r *fakeNodeDeleteTxRunner) WithNodeDeleteTx(_ context.Context, fn func(DeleteTx) error) error {
	r.called = true
	return fn(fakeNodeDeleteTx{repo: r.repo})
}

type fakeNodeDeleteTx struct {
	repo *fakeDeleteRepo
}

func (tx fakeNodeDeleteTx) NodeDeleteRepository() DeleteRepository {
	return tx.repo
}
