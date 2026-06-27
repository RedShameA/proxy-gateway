package nodes

import (
	"context"
	"errors"
	"testing"
)

func TestManualUpdateServiceInvalidatesOldFingerprintForInPlaceUpdate(t *testing.T) {
	oldOutbound := `{"type":"direct","tag":"old"}`
	updateRepo := &fakeManualUpdateRepo{
		currentEnabled: 1,
		manualSources:  1,
		totalSources:   1,
	}
	service := ManualUpdateService{
		Runner: &fakeManualUpdateTxRunner{repo: updateRepo},
		Nodes: &fakeManualUpdateReadRepository{records: map[string]Record{
			"node_1": {
				ID:           "node_1",
				Name:         "Old",
				Type:         "direct",
				OutboundJSON: oldOutbound,
			},
		}},
		NewNodeID: func() (string, error) {
			return "", errors.New("new node id should not be used for in-place update")
		},
	}

	result, err := service.Update(context.Background(), ManualUpdateCommand{
		NodeID: "node_1",
		Node: OutboundNode{
			Name: "New",
			Type: "direct",
		},
		NowMillis: 1000,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.NodeID != "node_1" || result.Split {
		t.Fatalf("result = %#v, want in-place update", result)
	}
	if len(result.InvalidatedFingerprints) != 1 || result.InvalidatedFingerprints[0] != OutboundFingerprint(oldOutbound) {
		t.Fatalf("invalidated fingerprints = %#v", result.InvalidatedFingerprints)
	}
}

func TestManualUpdateServiceDoesNotInvalidateOldFingerprintForSplitUpdate(t *testing.T) {
	updateRepo := &fakeManualUpdateRepo{
		currentEnabled: 1,
		manualSources:  1,
		totalSources:   2,
	}
	service := ManualUpdateService{
		Runner: &fakeManualUpdateTxRunner{repo: updateRepo},
		Nodes: &fakeManualUpdateReadRepository{records: map[string]Record{
			"node_shared": {
				ID:           "node_shared",
				Name:         "Shared",
				Type:         "direct",
				OutboundJSON: `{"type":"direct","tag":"old"}`,
			},
		}},
		NewNodeID: func() (string, error) {
			return "node_new", nil
		},
	}

	result, err := service.Update(context.Background(), ManualUpdateCommand{
		NodeID: "node_shared",
		Node: OutboundNode{
			Name: "New",
			Type: "direct",
		},
		NowMillis: 1000,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if result.NodeID != "node_new" || !result.Split {
		t.Fatalf("result = %#v, want split update", result)
	}
	if len(result.InvalidatedFingerprints) != 0 {
		t.Fatalf("split update invalidated old fingerprint: %#v", result.InvalidatedFingerprints)
	}
}

type fakeManualUpdateTxRunner struct {
	repo *fakeManualUpdateRepo
}

func (r *fakeManualUpdateTxRunner) WithManualUpdateTx(_ context.Context, fn func(ManualUpdateTx) error) error {
	return fn(fakeManualUpdateTx{repo: r.repo})
}

type fakeManualUpdateTx struct {
	repo *fakeManualUpdateRepo
}

func (tx fakeManualUpdateTx) NodeManualUpdateRepository() ManualUpdateRepository {
	return tx.repo
}

type fakeManualUpdateReadRepository struct {
	records map[string]Record
}

func (r *fakeManualUpdateReadRepository) Load(_ context.Context, id string) (Record, bool, error) {
	record, ok := r.records[id]
	return record, ok, nil
}

func (r *fakeManualUpdateReadRepository) ListIDs(context.Context, ListFilter) (ListResult, error) {
	return ListResult{}, nil
}

func (r *fakeManualUpdateReadRepository) ListEnabledObservationTargets(context.Context) ([]Record, error) {
	return nil, nil
}

func (r *fakeManualUpdateReadRepository) ListSources(context.Context, string) ([]SourceRecord, error) {
	return nil, nil
}

func (r *fakeManualUpdateReadRepository) LoadObservation(context.Context, string) (ObservationRecord, bool, error) {
	return ObservationRecord{}, false, nil
}

func (r *fakeManualUpdateReadRepository) SetEnabled(context.Context, string, bool) (bool, error) {
	return false, nil
}
