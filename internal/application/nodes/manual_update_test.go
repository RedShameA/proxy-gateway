package nodes

import (
	"errors"
	"testing"
)

func TestUpdateManualNodeEditsPureManualNodeInPlace(t *testing.T) {
	repo := &fakeManualUpdateRepo{
		currentEnabled: 1,
		manualSources:  1,
		totalSources:   1,
	}
	service := Service{
		NewNodeID: func() (string, error) {
			t.Fatal("NewNodeID should not be called for pure manual in-place update")
			return "", nil
		},
	}
	disabled := false

	result, err := service.UpdateManual(repo, ManualUpdateInput{
		NodeID:       "node-1",
		Fingerprint:  "fp-2",
		Name:         "manual-updated",
		Type:         "socks5",
		Server:       "127.0.0.2",
		ServerPort:   19084,
		Username:     "new-user",
		Password:     "new-pass",
		OutboundJSON: `{"type":"socks"}`,
		Enabled:      &disabled,
	})
	if err != nil {
		t.Fatalf("UpdateManual error = %v", err)
	}
	if result.NodeID != "node-1" || result.Split {
		t.Fatalf("result = %#v, want same node without split", result)
	}
	if repo.updated == nil {
		t.Fatal("expected node update")
	}
	if repo.updated.Enabled != 0 || repo.updated.Name != "manual-updated" || repo.updated.Type != "socks5" {
		t.Fatalf("updated record = %#v", repo.updated)
	}
	if repo.displayName == nil || repo.displayName.nodeID != "node-1" || repo.displayName.name != "manual-updated" {
		t.Fatalf("display name update = %#v", repo.displayName)
	}
	if repo.deletedManualSourceNodeID != "" {
		t.Fatalf("delete manual source called for in-place update: %#v", repo)
	}
}

func TestUpdateManualNodeSplitsSharedNodeAndRebindsManualSource(t *testing.T) {
	repo := &fakeManualUpdateRepo{
		currentEnabled: 1,
		manualSources:  1,
		totalSources:   2,
	}
	service := Service{
		NewNodeID: func() (string, error) {
			return "node-new", nil
		},
	}
	enabled := false

	result, err := service.UpdateManual(repo, ManualUpdateInput{
		NodeID:       "node-shared",
		Fingerprint:  "fp-new",
		Name:         "manual-split",
		Type:         "http",
		Server:       "127.0.0.2",
		ServerPort:   19086,
		Username:     "split-user",
		Password:     "split-pass",
		OutboundJSON: `{"type":"http"}`,
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatalf("UpdateManual error = %v", err)
	}
	if result.NodeID != "node-new" || !result.Split {
		t.Fatalf("result = %#v, want new split node", result)
	}
	if repo.deletedManualSourceNodeID != "node-shared" {
		t.Fatalf("deletedManualSourceNodeID = %q, want node-shared", repo.deletedManualSourceNodeID)
	}
	if repo.created == nil || repo.bound == nil {
		t.Fatalf("expected upsert create+bind, repo=%#v", repo)
	}
	if repo.bound.NodeID != "node-new" || repo.bound.SourceType != "manual" {
		t.Fatalf("source binding = %#v", repo.bound)
	}
	if repo.setEnabled == nil || repo.setEnabled.nodeID != "node-new" || repo.setEnabled.enabled != 0 {
		t.Fatalf("setEnabled = %#v, want node-new disabled", repo.setEnabled)
	}
}

func TestUpdateManualNodeRejectsDuplicateForPureManualNode(t *testing.T) {
	repo := &fakeManualUpdateRepo{
		currentEnabled: 1,
		manualSources:  1,
		totalSources:   1,
		duplicateID:    "node-other",
	}
	service := Service{}

	_, err := service.UpdateManual(repo, ManualUpdateInput{
		NodeID:      "node-1",
		Fingerprint: "fp-duplicate",
		Name:        "manual-duplicate",
		Type:        "http",
	})
	if !errors.Is(err, ErrDuplicateNode) {
		t.Fatalf("UpdateManual error = %v, want ErrDuplicateNode", err)
	}
	if repo.updated != nil || repo.created != nil || repo.bound != nil {
		t.Fatalf("repo mutated on duplicate: %#v", repo)
	}
}

type fakeManualUpdateRepo struct {
	existingID                string
	currentEnabled            int
	manualSources             int
	totalSources              int
	duplicateID               string
	updated                   *UpdateNodeRecord
	displayName               *displayNameUpdate
	deletedManualSourceNodeID string
	setEnabled                *setEnabledRecord
	created                   *CreateNodeRecord
	bound                     *BindSourceRecord
}

type displayNameUpdate struct {
	nodeID string
	name   string
}

type setEnabledRecord struct {
	nodeID  string
	enabled int
}

func (f *fakeManualUpdateRepo) CurrentNodeEnabled(nodeID string) (int, error) {
	return f.currentEnabled, nil
}

func (f *fakeManualUpdateRepo) NodeSourceCounts(nodeID string) (int, int, error) {
	return f.manualSources, f.totalSources, nil
}

func (f *fakeManualUpdateRepo) FindNodeIDByFingerprint(fingerprint string) (string, error) {
	return f.existingID, nil
}

func (f *fakeManualUpdateRepo) FindOtherNodeIDByFingerprint(fingerprint, excludeNodeID string) (string, error) {
	return f.duplicateID, nil
}

func (f *fakeManualUpdateRepo) UpdateNode(record UpdateNodeRecord) error {
	f.updated = &record
	return nil
}

func (f *fakeManualUpdateRepo) UpdateManualSourceDisplayName(nodeID, name string) error {
	f.displayName = &displayNameUpdate{nodeID: nodeID, name: name}
	return nil
}

func (f *fakeManualUpdateRepo) DeleteManualSource(nodeID string) error {
	f.deletedManualSourceNodeID = nodeID
	return nil
}

func (f *fakeManualUpdateRepo) SetNodeEnabled(nodeID string, enabled int) error {
	f.setEnabled = &setEnabledRecord{nodeID: nodeID, enabled: enabled}
	return nil
}

func (f *fakeManualUpdateRepo) CreateNode(record CreateNodeRecord) error {
	f.created = &record
	return nil
}

func (f *fakeManualUpdateRepo) BindNodeSource(record BindSourceRecord) error {
	f.bound = &record
	return nil
}
