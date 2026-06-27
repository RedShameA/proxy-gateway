package subscriptions

import (
	"context"
	"testing"
)

func TestDeleteServiceRunsSubscriptionDeleteInTransaction(t *testing.T) {
	repo := &fakeDeleteRepo{
		affected:            1,
		nodeIDs:             []string{"node-1"},
		deletedFingerprints: []string{"fp-1"},
	}
	runner := &fakeSubscriptionDeleteTxRunner{repo: repo}
	service := DeleteService{
		Runner: runner,
		Now: func() int64 {
			return 123
		},
	}

	result, err := service.Delete(context.Background(), "sub-1")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !runner.called || runner.nowMillis != 123 {
		t.Fatalf("runner called=%t nowMillis=%d", runner.called, runner.nowMillis)
	}
	if repo.deletedSubscriptionID != "sub-1" {
		t.Fatalf("deleted subscription id = %q", repo.deletedSubscriptionID)
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp-1" {
		t.Fatalf("result = %#v", result)
	}
}

type fakeSubscriptionDeleteTxRunner struct {
	repo      *fakeDeleteRepo
	called    bool
	nowMillis int64
}

func (r *fakeSubscriptionDeleteTxRunner) WithSubscriptionDeleteTx(_ context.Context, fn func(DeleteTx) error) error {
	r.called = true
	return fn(fakeSubscriptionDeleteTx{runner: r})
}

type fakeSubscriptionDeleteTx struct {
	runner *fakeSubscriptionDeleteTxRunner
}

func (tx fakeSubscriptionDeleteTx) SubscriptionSourceRepository(nowMillis int64) SourceRepository {
	tx.runner.nowMillis = nowMillis
	return tx.runner.repo
}

func (f *fakeDeleteRepo) ExistingSourceNodeIDs(string) ([]string, error) {
	return nil, nil
}

func (f *fakeDeleteRepo) DeleteSubscriptionNodeSource(string, string) error {
	return nil
}

func (f *fakeDeleteRepo) RetainStickyProfilesForRemovedNode(string) ([]StickyProfileEvaluationRef, error) {
	return nil, nil
}

func (f *fakeDeleteRepo) RetainedStickyProfilesForRefresh() ([]StickyProfileEvaluationRef, error) {
	return nil, nil
}
