package nodes

import (
	"context"
	"testing"
)

func TestCreateServiceBuildsInputAndUpsertsNodeInTransaction(t *testing.T) {
	repo := &fakeCreateUpsertRepository{}
	runner := &fakeCreateTxRunner{repo: repo}
	service := CreateService{
		Runner: runner,
		NewNodeID: func() (string, error) {
			return "node_created", nil
		},
	}

	nodeID, err := service.Create(context.Background(), CreateCommand{
		Node: OutboundNode{
			Name:       "Manual",
			Type:       "http",
			Server:     "127.0.0.1",
			ServerPort: 8080,
		},
		Source:    SourceInput{ID: "manual", Name: "Manual", Type: "manual"},
		NowMillis: 123,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !runner.called {
		t.Fatal("expected create service to use transaction runner")
	}
	if nodeID != "node_created" {
		t.Fatalf("nodeID = %q", nodeID)
	}
	if len(repo.created) != 1 || repo.created[0].Name != "Manual" || repo.created[0].Type != "http" {
		t.Fatalf("created = %#v", repo.created)
	}
	if len(repo.bound) != 1 || repo.bound[0].SourceID != "manual" || repo.bound[0].SourceType != "manual" {
		t.Fatalf("bound = %#v", repo.bound)
	}
}

type fakeCreateTxRunner struct {
	repo   *fakeCreateUpsertRepository
	called bool
}

func (r *fakeCreateTxRunner) WithNodeUpsertTx(_ context.Context, fn func(UpsertTx) error) error {
	r.called = true
	return fn(fakeCreateTx{repo: r.repo})
}

type fakeCreateTx struct {
	repo *fakeCreateUpsertRepository
}

func (tx fakeCreateTx) NodeUpsertRepository() UpsertRepository {
	return tx.repo
}

type fakeCreateUpsertRepository struct {
	existingID string
	created    []CreateNodeRecord
	bound      []BindSourceRecord
}

func (r *fakeCreateUpsertRepository) FindNodeIDByFingerprint(string) (string, error) {
	return r.existingID, nil
}

func (r *fakeCreateUpsertRepository) CreateNode(record CreateNodeRecord) error {
	r.created = append(r.created, record)
	return nil
}

func (r *fakeCreateUpsertRepository) BindNodeSource(record BindSourceRecord) error {
	r.bound = append(r.bound, record)
	return nil
}
