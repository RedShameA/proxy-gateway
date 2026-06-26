package subscriptions

import (
	"errors"
	"testing"
)

func TestDeleteSubscriptionSourceRemovesSourceAndCleansOrphans(t *testing.T) {
	repo := &fakeDeleteRepo{
		affected:            1,
		nodeIDs:             []string{"node-1", "node-2"},
		deletedFingerprints: []string{"fp-1"},
	}

	result, err := DeleteSubscriptionSource(repo, "sub-1")
	if err != nil {
		t.Fatalf("DeleteSubscriptionSource error = %v", err)
	}
	if repo.deletedSubscriptionID != "sub-1" || repo.deletedSourcesSubscriptionID != "sub-1" || repo.invalidatedSubscriptionID != "sub-1" {
		t.Fatalf("repo calls = %#v", repo)
	}
	if len(repo.cleanupNodeIDs) != 2 || repo.cleanupNodeIDs[0] != "node-1" || repo.cleanupNodeIDs[1] != "node-2" {
		t.Fatalf("cleanup node ids = %#v", repo.cleanupNodeIDs)
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp-1" {
		t.Fatalf("DeletedFingerprints = %#v", result.DeletedFingerprints)
	}
}

func TestDeleteSubscriptionSourceRejectsMissingSubscription(t *testing.T) {
	repo := &fakeDeleteRepo{}

	_, err := DeleteSubscriptionSource(repo, "sub-missing")
	if !errors.Is(err, ErrSubscriptionNotFound) {
		t.Fatalf("DeleteSubscriptionSource error = %v, want ErrSubscriptionNotFound", err)
	}
	if repo.nodeIDs != nil || repo.cleanupNodeIDs != nil {
		t.Fatalf("unexpected follow-up work for missing subscription: %#v", repo)
	}
}

type fakeDeleteRepo struct {
	affected                     int64
	nodeIDs                      []string
	deletedFingerprints          []string
	deletedSubscriptionID        string
	deletedSourcesSubscriptionID string
	invalidatedSubscriptionID    string
	cleanupNodeIDs               []string
}

func (f *fakeDeleteRepo) DeleteSubscription(subscriptionID string) (int64, error) {
	f.deletedSubscriptionID = subscriptionID
	return f.affected, nil
}

func (f *fakeDeleteRepo) NodeIDsForSource(subscriptionID, sourceType string) ([]string, error) {
	return append([]string{}, f.nodeIDs...), nil
}

func (f *fakeDeleteRepo) DeleteSubscriptionNodeSources(subscriptionID string) error {
	f.deletedSourcesSubscriptionID = subscriptionID
	return nil
}

func (f *fakeDeleteRepo) InvalidateProfilesForDeletedSubscription(subscriptionID string) error {
	f.invalidatedSubscriptionID = subscriptionID
	return nil
}

func (f *fakeDeleteRepo) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	f.cleanupNodeIDs = append([]string{}, nodeIDs...)
	return append([]string{}, f.deletedFingerprints...), nil
}
