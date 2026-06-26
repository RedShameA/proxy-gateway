package subscriptions

import "testing"

func TestPruneRefreshSnapshotRemovesStaleSourcesAndDedupesStickyProfiles(t *testing.T) {
	repo := &fakeRefreshSnapshotRepo{
		existingNodeIDs: []string{"node-stale", "node-keep"},
		removedSticky: map[string][]StickyProfileEvaluationRef{
			"node-stale": {
				{ID: "profile-1", Name: "p1", ConfigVersion: 1},
			},
		},
		retainedSticky: []StickyProfileEvaluationRef{
			{ID: "profile-1", Name: "p1", ConfigVersion: 1},
			{ID: "profile-2", Name: "p2", ConfigVersion: 2},
		},
		deletedFingerprints: []string{"fp-stale"},
	}

	result, err := PruneRefreshSnapshot(repo, "sub-1", []string{"node-keep"})
	if err != nil {
		t.Fatalf("PruneRefreshSnapshot error = %v", err)
	}
	if len(repo.deletedSourceNodeIDs) != 1 || repo.deletedSourceNodeIDs[0] != "node-stale" {
		t.Fatalf("deleted source node ids = %#v", repo.deletedSourceNodeIDs)
	}
	if len(repo.cleanupNodeIDs) != 1 || repo.cleanupNodeIDs[0] != "node-stale" {
		t.Fatalf("cleanup node ids = %#v", repo.cleanupNodeIDs)
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp-stale" {
		t.Fatalf("DeletedFingerprints = %#v", result.DeletedFingerprints)
	}
	if len(result.StickyProfilesToEvaluate) != 2 || result.StickyProfilesToEvaluate[0].ID != "profile-1" || result.StickyProfilesToEvaluate[1].ID != "profile-2" {
		t.Fatalf("StickyProfilesToEvaluate = %#v", result.StickyProfilesToEvaluate)
	}
}

type fakeRefreshSnapshotRepo struct {
	existingNodeIDs      []string
	removedSticky        map[string][]StickyProfileEvaluationRef
	retainedSticky       []StickyProfileEvaluationRef
	deletedFingerprints  []string
	deletedSourceNodeIDs []string
	cleanupNodeIDs       []string
}

func (f *fakeRefreshSnapshotRepo) ExistingSourceNodeIDs(subscriptionID string) ([]string, error) {
	return append([]string{}, f.existingNodeIDs...), nil
}

func (f *fakeRefreshSnapshotRepo) DeleteSubscriptionNodeSource(nodeID, subscriptionID string) error {
	f.deletedSourceNodeIDs = append(f.deletedSourceNodeIDs, nodeID)
	return nil
}

func (f *fakeRefreshSnapshotRepo) RetainStickyProfilesForRemovedNode(nodeID string) ([]StickyProfileEvaluationRef, error) {
	return append([]StickyProfileEvaluationRef{}, f.removedSticky[nodeID]...), nil
}

func (f *fakeRefreshSnapshotRepo) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	f.cleanupNodeIDs = append([]string{}, nodeIDs...)
	return append([]string{}, f.deletedFingerprints...), nil
}

func (f *fakeRefreshSnapshotRepo) RetainedStickyProfilesForRefresh() ([]StickyProfileEvaluationRef, error) {
	return append([]StickyProfileEvaluationRef{}, f.retainedSticky...), nil
}
