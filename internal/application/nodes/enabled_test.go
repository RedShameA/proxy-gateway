package nodes

import (
	"context"
	"errors"
	"testing"
)

func TestSetEnabledDisablesNodeAndReturnsRuntimeFingerprint(t *testing.T) {
	repo := &fakeEnabledRepo{
		node:  Record{ID: "node_1", Type: "http", Server: "127.0.0.1", ServerPort: 18080, OutboundJSON: `{"type":"http","server":"127.0.0.1","server_port":18080}`},
		found: true,
	}

	result, err := SetEnabled(context.Background(), repo, "node_1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.RuntimeFingerprint == "" {
		t.Fatalf("result = %#v", result)
	}
	if repo.setEnabledNodeID != "node_1" || repo.setEnabledValue {
		t.Fatalf("set enabled call = %q %t", repo.setEnabledNodeID, repo.setEnabledValue)
	}
}

func TestSetEnabledEnablesNodeWithoutLoadingFingerprint(t *testing.T) {
	repo := &fakeEnabledRepo{found: true}

	result, err := SetEnabled(context.Background(), repo, "node_1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated || result.RuntimeFingerprint != "" {
		t.Fatalf("result = %#v", result)
	}
	if repo.loadCalled {
		t.Fatal("Load should not be called when enabling node")
	}
}

func TestSetEnabledReturnsNotFound(t *testing.T) {
	repo := &fakeEnabledRepo{}

	_, err := SetEnabled(context.Background(), repo, "node_missing", false)
	if !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("SetEnabled error = %v, want ErrNodeNotFound", err)
	}
}

type fakeEnabledRepo struct {
	node             Record
	found            bool
	loadCalled       bool
	setEnabledNodeID string
	setEnabledValue  bool
}

func (r *fakeEnabledRepo) Load(_ context.Context, _ string) (Record, bool, error) {
	r.loadCalled = true
	return r.node, r.found, nil
}

func (r *fakeEnabledRepo) ListIDs(context.Context, ListFilter) (ListResult, error) {
	return ListResult{}, nil
}

func (r *fakeEnabledRepo) ListEnabledObservationTargets(context.Context) ([]Record, error) {
	return nil, nil
}

func (r *fakeEnabledRepo) ListSources(context.Context, string) ([]SourceRecord, error) {
	return nil, nil
}

func (r *fakeEnabledRepo) LoadObservation(context.Context, string) (ObservationRecord, bool, error) {
	return ObservationRecord{}, false, nil
}

func (r *fakeEnabledRepo) SetEnabled(_ context.Context, nodeID string, enabled bool) (bool, error) {
	r.setEnabledNodeID = nodeID
	r.setEnabledValue = enabled
	return r.found, nil
}
