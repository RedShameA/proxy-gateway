package profiles

import (
	"context"
	"testing"
)

func TestUpdateConfigWithReleaseUpdatesConfigThenReleasesRetainedNodes(t *testing.T) {
	repo := newFakeConfigRepository()
	runner := &fakeConfigReleaseTxRunner{
		repo:                repo,
		deletedFingerprints: []string{"fp-1"},
	}
	update := UpdateConfigWithRelease(runner)
	cfg := DefaultConfig("profile_1")
	cfg.Name = "Updated"

	deleted, err := update(context.Background(), "profile_1", cfg, ConfigUpdateOptions{ResetCurrentPath: true})
	if err != nil {
		t.Fatalf("update() error = %v", err)
	}
	if !runner.called {
		t.Fatal("expected update to use transaction runner")
	}
	if repo.updated.Name != "Updated" || !repo.options.ResetCurrentPath {
		t.Fatalf("updated config = %#v options=%#v", repo.updated, repo.options)
	}
	if runner.releasedProfileID != "profile_1" {
		t.Fatalf("released profile id = %q", runner.releasedProfileID)
	}
	if len(deleted) != 1 || deleted[0] != "fp-1" {
		t.Fatalf("deleted fingerprints = %#v", deleted)
	}
}

type fakeConfigReleaseTxRunner struct {
	repo                *fakeConfigRepository
	deletedFingerprints []string
	called              bool
	releasedProfileID   string
}

func (r *fakeConfigReleaseTxRunner) WithProfileConfigReleaseTx(_ context.Context, fn func(ConfigReleaseTx) error) error {
	r.called = true
	return fn(fakeConfigReleaseTx{runner: r})
}

type fakeConfigReleaseTx struct {
	runner *fakeConfigReleaseTxRunner
}

func (tx fakeConfigReleaseTx) ProfileConfigRepository() ConfigUpdater {
	return tx.runner.repo
}

func (tx fakeConfigReleaseTx) ReleaseRetainedProfileNodesExcept(profileID string, _ []string) ([]string, error) {
	tx.runner.releasedProfileID = profileID
	return append([]string{}, tx.runner.deletedFingerprints...), nil
}
