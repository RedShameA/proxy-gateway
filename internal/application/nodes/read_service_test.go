package nodes

import (
	"context"
	"errors"
	"testing"
)

func TestReadServiceListsAndLoadsNodeDetails(t *testing.T) {
	repo := &fakeReadRepository{
		ids: []string{"node_1"},
		nodes: map[string]Record{
			"node_1": {ID: "node_1", Name: "Node 1", Type: "http", Server: "127.0.0.1", ServerPort: 18080, Enabled: true},
		},
		sources: map[string][]SourceRecord{
			"node_1": {{SourceID: "manual", SourceName: "Manual", SourceType: "manual", DisplayName: "Node 1"}},
		},
		observations: map[string]ObservationRecord{
			"node_1": {Usable: true, EgressIP: "203.0.113.10", EgressCountry: "JP", LatencyMS: 42, LastSuccessAt: 1000},
		},
	}
	service := NewReadService(repo)

	list, err := service.List(context.Background(), ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list["total"] != 1 {
		t.Fatalf("list total = %#v, want 1", list["total"])
	}
	items := list["items"].([]map[string]any)
	nodes := list["nodes"].([]map[string]any)
	if len(items) != 1 || len(nodes) != 1 || items[0]["id"] != "node_1" || nodes[0]["id"] != "node_1" {
		t.Fatalf("list aliases = %#v / %#v", items, nodes)
	}
	if items[0]["state"] != "usable" || items[0]["egress_country"] != "JP" {
		t.Fatalf("list item = %#v", items[0])
	}

	detail, err := service.Detail(context.Background(), "node_1")
	if err != nil {
		t.Fatal(err)
	}
	if detail["id"] != "node_1" || detail["raw_json"] != "" {
		t.Fatalf("detail = %#v", detail)
	}
	sources := detail["sources"].([]map[string]any)
	if len(sources) != 1 || sources[0]["source_id"] != "manual" {
		t.Fatalf("detail sources = %#v", sources)
	}
}

func TestReadServiceReturnsNodeNotFound(t *testing.T) {
	service := NewReadService(&fakeReadRepository{nodes: map[string]Record{}})

	_, err := service.Detail(context.Background(), "missing")
	if !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("Detail error = %v, want ErrNodeNotFound", err)
	}
}

type fakeReadRepository struct {
	ids          []string
	nodes        map[string]Record
	sources      map[string][]SourceRecord
	observations map[string]ObservationRecord
}

func (r *fakeReadRepository) Load(_ context.Context, id string) (Record, bool, error) {
	node, ok := r.nodes[id]
	return node, ok, nil
}

func (r *fakeReadRepository) ListIDs(context.Context, ListFilter) (ListResult, error) {
	return ListResult{IDs: append([]string{}, r.ids...), Total: len(r.ids)}, nil
}

func (r *fakeReadRepository) ListEnabledObservationTargets(context.Context) ([]Record, error) {
	return nil, nil
}

func (r *fakeReadRepository) ListSources(_ context.Context, nodeID string) ([]SourceRecord, error) {
	return append([]SourceRecord{}, r.sources[nodeID]...), nil
}

func (r *fakeReadRepository) LoadObservation(_ context.Context, nodeID string) (ObservationRecord, bool, error) {
	observation, ok := r.observations[nodeID]
	return observation, ok, nil
}

func (r *fakeReadRepository) SetEnabled(context.Context, string, bool) (bool, error) {
	return false, nil
}
