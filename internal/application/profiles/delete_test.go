package profiles

import (
	"errors"
	"testing"
)

func TestDeleteProfileRemovesRelatedRowsAndCleansRetainedNodes(t *testing.T) {
	repo := &fakeProfileDeleteRepo{
		affected:            1,
		retainedNodeIDs:     []string{"node_1", "node_2"},
		deletedFingerprints: []string{"fp_1"},
	}

	result, err := DeleteProfile(repo, "profile_1")
	if err != nil {
		t.Fatalf("DeleteProfile error = %v", err)
	}
	if repo.credentialsProfileID != "profile_1" || repo.runsProfileID != "profile_1" || repo.retainedProfileID != "profile_1" || repo.profileID != "profile_1" {
		t.Fatalf("repo calls = %#v", repo)
	}
	if len(repo.cleanupNodeIDs) != 2 || repo.cleanupNodeIDs[0] != "node_1" || repo.cleanupNodeIDs[1] != "node_2" {
		t.Fatalf("cleanupNodeIDs = %#v", repo.cleanupNodeIDs)
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp_1" {
		t.Fatalf("DeletedFingerprints = %#v", result.DeletedFingerprints)
	}
}

func TestDeleteProfileRejectsMissingProfile(t *testing.T) {
	repo := &fakeProfileDeleteRepo{}

	_, err := DeleteProfile(repo, "profile_missing")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("DeleteProfile error = %v, want ErrProfileNotFound", err)
	}
	if repo.cleanupNodeIDs != nil {
		t.Fatalf("cleanup should not be called for missing profile: %#v", repo.cleanupNodeIDs)
	}
}

type fakeProfileDeleteRepo struct {
	affected             int64
	retainedNodeIDs      []string
	deletedFingerprints  []string
	credentialsProfileID string
	runsProfileID        string
	retainedProfileID    string
	profileID            string
	cleanupNodeIDs       []string
}

func (f *fakeProfileDeleteRepo) DeleteCredentials(profileID string) error {
	f.credentialsProfileID = profileID
	return nil
}

func (f *fakeProfileDeleteRepo) DeleteMaintenanceRuns(profileID string) error {
	f.runsProfileID = profileID
	return nil
}

func (f *fakeProfileDeleteRepo) RetainedNodeIDs(profileID string) ([]string, error) {
	f.retainedProfileID = profileID
	return append([]string{}, f.retainedNodeIDs...), nil
}

func (f *fakeProfileDeleteRepo) DeleteRetainedNodes(profileID string) error {
	return nil
}

func (f *fakeProfileDeleteRepo) DeleteProfile(profileID string) (int64, error) {
	f.profileID = profileID
	return f.affected, nil
}

func (f *fakeProfileDeleteRepo) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	f.cleanupNodeIDs = append([]string{}, nodeIDs...)
	return append([]string{}, f.deletedFingerprints...), nil
}
