package profiles

import (
	"context"
	"testing"
)

func TestDeleteServiceRunsProfileDeleteInTransaction(t *testing.T) {
	repo := &fakeProfileDeleteRepo{
		affected:            1,
		retainedNodeIDs:     []string{"node_1"},
		deletedFingerprints: []string{"fp_1"},
	}
	runner := &fakeProfileDeleteTxRunner{repo: repo}
	service := DeleteService{Runner: runner}

	result, err := service.Delete(context.Background(), "profile_1")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !runner.called {
		t.Fatal("expected delete service to use transaction runner")
	}
	if repo.profileID != "profile_1" {
		t.Fatalf("deleted profile id = %q", repo.profileID)
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp_1" {
		t.Fatalf("result = %#v", result)
	}
}

type fakeProfileDeleteTxRunner struct {
	repo   *fakeProfileDeleteRepo
	called bool
}

func (r *fakeProfileDeleteTxRunner) WithProfileDeleteTx(_ context.Context, fn func(DeleteTx) error) error {
	r.called = true
	return fn(fakeProfileDeleteTx{repo: r.repo})
}

type fakeProfileDeleteTx struct {
	repo *fakeProfileDeleteRepo
}

func (tx fakeProfileDeleteTx) ProfileDeleteRepository() DeleteRepository {
	return tx.repo
}
