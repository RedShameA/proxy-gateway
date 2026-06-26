package nodes

import (
	"errors"
	"testing"
)

func TestDeleteManualSourceCleansUpOrphanedNode(t *testing.T) {
	repo := &fakeDeleteRepo{
		affected:            1,
		deletedFingerprints: []string{"fp-1"},
	}

	result, err := DeleteManualSource(repo, "node-1")
	if err != nil {
		t.Fatalf("DeleteManualSource error = %v", err)
	}
	if repo.deletedNodeID != "node-1" {
		t.Fatalf("deletedNodeID = %q, want node-1", repo.deletedNodeID)
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp-1" {
		t.Fatalf("DeletedFingerprints = %#v", result.DeletedFingerprints)
	}
}

func TestDeleteManualSourceRejectsMissingManualSource(t *testing.T) {
	repo := &fakeDeleteRepo{}

	_, err := DeleteManualSource(repo, "node-missing")
	if !errors.Is(err, ErrManualNodeSourceMissing) {
		t.Fatalf("DeleteManualSource error = %v, want ErrManualNodeSourceMissing", err)
	}
	if repo.cleanupNodeIDs != nil {
		t.Fatalf("cleanup should not be called: %#v", repo.cleanupNodeIDs)
	}
}

type fakeDeleteRepo struct {
	affected            int64
	deletedNodeID       string
	cleanupNodeIDs      []string
	deletedFingerprints []string
}

func (f *fakeDeleteRepo) DeleteManualSource(nodeID string) (int64, error) {
	f.deletedNodeID = nodeID
	return f.affected, nil
}

func (f *fakeDeleteRepo) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	f.cleanupNodeIDs = append([]string{}, nodeIDs...)
	return append([]string{}, f.deletedFingerprints...), nil
}
