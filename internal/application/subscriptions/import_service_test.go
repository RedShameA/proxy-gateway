package subscriptions

import (
	"context"
	"errors"
	"testing"

	appnodes "proxygateway/internal/application/nodes"
)

func TestImportServiceCreatesImportAndUpsertsNodes(t *testing.T) {
	tx := &fakeImportTx{
		upserts: fakeImportUpsertRepository{},
	}
	runner := &fakeImportTxRunner{tx: tx}
	service := ImportService{
		Runner: runner,
		NewNodeID: func() (string, error) {
			return "node_1", nil
		},
	}

	result, snapshot, err := service.Import(context.Background(), ImportCommand{
		SubscriptionID:             "sub_1",
		Name:                       "Remote",
		SourceType:                 "remote",
		URL:                        "https://example.test/sub",
		Content:                    "content",
		AutoRefreshEnabled:         true,
		AutoRefreshIntervalSeconds: 3600,
		NowMillis:                  123,
	}, ParsedImportContent{
		Nodes: []ParsedNode{{
			Name: "Direct",
			Type: "direct",
		}},
		SkippedEntries:     2,
		SkippedSummary:     []SkippedEntrySummary{{Reason: "unsupported", Count: 2, Message: "unsupported"}},
		SkippedSummaryJSON: `[{"reason":"unsupported","count":2,"message":"unsupported"}]`,
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if !runner.called {
		t.Fatal("expected import to run in transaction")
	}
	if tx.imports.created.ID != "sub_1" || tx.imports.created.ImportedNodes != 1 || tx.imports.created.SkippedEntries != 2 {
		t.Fatalf("created import record = %#v", tx.imports.created)
	}
	if tx.imports.refreshed.ID != "" {
		t.Fatalf("unexpected refresh import record = %#v", tx.imports.refreshed)
	}
	if len(tx.upserts.created) != 1 || tx.upserts.created[0].ID != "node_1" {
		t.Fatalf("created nodes = %#v", tx.upserts.created)
	}
	if len(tx.upserts.bound) != 1 || tx.upserts.bound[0].SourceID != "sub_1" || tx.upserts.bound[0].SourceType != "subscription" {
		t.Fatalf("bound sources = %#v", tx.upserts.bound)
	}
	if result.ID != "sub_1" || result.ImportedNodes != 1 || result.SkippedEntries != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(snapshot.DeletedFingerprints) != 0 || len(snapshot.StickyProfilesToEvaluate) != 0 {
		t.Fatalf("create import should not prune refresh snapshot: %#v", snapshot)
	}
}

func TestImportServiceRefreshPrunesStaleNodes(t *testing.T) {
	tx := &fakeImportTx{
		upserts: fakeImportUpsertRepository{findID: "node_current"},
		sources: fakeImportSourceRepository{
			existingNodeIDs:       []string{"node_stale", "node_current"},
			deletedFingerprints:   []string{"fingerprint_stale"},
			stickyProfilesForNode: map[string][]StickyProfileEvaluationRef{"node_stale": {{ID: "profile_1", Name: "Profile", ConfigVersion: 7}}},
		},
	}
	service := ImportService{
		Runner: &fakeImportTxRunner{tx: tx},
		NewNodeID: func() (string, error) {
			return "", errors.New("new node id should not be used when fingerprint already exists")
		},
	}

	result, snapshot, err := service.Import(context.Background(), ImportCommand{
		SubscriptionID: "sub_1",
		Name:           "Remote",
		SourceType:     "remote",
		Content:        "content",
		Refresh:        true,
		NowMillis:      456,
	}, ParsedImportContent{
		Nodes: []ParsedNode{{Name: "Direct", Type: "direct"}},
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if tx.imports.refreshed.ID != "sub_1" || tx.imports.created.ID != "" {
		t.Fatalf("import records created=%#v refreshed=%#v", tx.imports.created, tx.imports.refreshed)
	}
	if len(tx.sources.deletedSources) != 1 || tx.sources.deletedSources[0] != "node_stale:sub_1" {
		t.Fatalf("deleted source refs = %#v", tx.sources.deletedSources)
	}
	if len(tx.sources.cleanedNodeIDs) != 1 || tx.sources.cleanedNodeIDs[0] != "node_stale" {
		t.Fatalf("cleaned node ids = %#v", tx.sources.cleanedNodeIDs)
	}
	if len(snapshot.DeletedFingerprints) != 1 || snapshot.DeletedFingerprints[0] != "fingerprint_stale" {
		t.Fatalf("snapshot deleted fingerprints = %#v", snapshot)
	}
	if len(result.StickyProfilesToEvaluate) != 1 || result.StickyProfilesToEvaluate[0].ID != "profile_1" {
		t.Fatalf("sticky refs in result = %#v", result.StickyProfilesToEvaluate)
	}
}

type fakeImportTxRunner struct {
	tx     *fakeImportTx
	called bool
}

func (r *fakeImportTxRunner) WithImportTx(_ context.Context, fn func(ImportTx) error) error {
	r.called = true
	return fn(r.tx)
}

type fakeImportTx struct {
	imports fakeImportRepository
	upserts fakeImportUpsertRepository
	sources fakeImportSourceRepository
}

func (tx *fakeImportTx) NodeUpsertRepository() appnodes.UpsertRepository {
	return &tx.upserts
}

func (tx *fakeImportTx) SubscriptionImportRepository() ImportRepository {
	return &tx.imports
}

func (tx *fakeImportTx) SubscriptionSourceRepository(int64) SourceRepository {
	return &tx.sources
}

type fakeImportRepository struct {
	created   ImportRecord
	refreshed ImportRecord
}

func (r *fakeImportRepository) CreateImport(record ImportRecord) error {
	r.created = record
	return nil
}

func (r *fakeImportRepository) RefreshImport(record ImportRecord) error {
	r.refreshed = record
	return nil
}

type fakeImportUpsertRepository struct {
	findID  string
	created []appnodes.CreateNodeRecord
	bound   []appnodes.BindSourceRecord
}

func (r *fakeImportUpsertRepository) FindNodeIDByFingerprint(string) (string, error) {
	return r.findID, nil
}

func (r *fakeImportUpsertRepository) CreateNode(record appnodes.CreateNodeRecord) error {
	r.created = append(r.created, record)
	return nil
}

func (r *fakeImportUpsertRepository) BindNodeSource(record appnodes.BindSourceRecord) error {
	r.bound = append(r.bound, record)
	return nil
}

type fakeImportSourceRepository struct {
	existingNodeIDs       []string
	deletedSources        []string
	stickyProfilesForNode map[string][]StickyProfileEvaluationRef
	cleanedNodeIDs        []string
	deletedFingerprints   []string
	retainedSticky        []StickyProfileEvaluationRef
}

func (r *fakeImportSourceRepository) DeleteSubscription(string) (int64, error) {
	return 0, nil
}

func (r *fakeImportSourceRepository) NodeIDsForSource(string, string) ([]string, error) {
	return nil, nil
}

func (r *fakeImportSourceRepository) DeleteSubscriptionNodeSources(string) error {
	return nil
}

func (r *fakeImportSourceRepository) InvalidateProfilesForDeletedSubscription(string) error {
	return nil
}

func (r *fakeImportSourceRepository) ExistingSourceNodeIDs(string) ([]string, error) {
	return r.existingNodeIDs, nil
}

func (r *fakeImportSourceRepository) DeleteSubscriptionNodeSource(nodeID, subscriptionID string) error {
	r.deletedSources = append(r.deletedSources, nodeID+":"+subscriptionID)
	return nil
}

func (r *fakeImportSourceRepository) RetainStickyProfilesForRemovedNode(nodeID string) ([]StickyProfileEvaluationRef, error) {
	return r.stickyProfilesForNode[nodeID], nil
}

func (r *fakeImportSourceRepository) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	r.cleanedNodeIDs = append(r.cleanedNodeIDs, nodeIDs...)
	return r.deletedFingerprints, nil
}

func (r *fakeImportSourceRepository) RetainedStickyProfilesForRefresh() ([]StickyProfileEvaluationRef, error) {
	return r.retainedSticky, nil
}
