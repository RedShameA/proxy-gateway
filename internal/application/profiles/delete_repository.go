package profiles

import "errors"

var ErrProfileNotFound = errors.New("profile not found")

type DeleteRepository interface {
	DeleteCredentials(profileID string) error
	DeleteMaintenanceRuns(profileID string) error
	RetainedNodeIDs(profileID string) ([]string, error)
	DeleteRetainedNodes(profileID string) error
	DeleteProfile(profileID string) (int64, error)
	CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error)
}

type DeleteResult struct {
	DeletedFingerprints []string
}

func DeleteProfile(repo DeleteRepository, profileID string) (DeleteResult, error) {
	if err := repo.DeleteCredentials(profileID); err != nil {
		return DeleteResult{}, err
	}
	if err := repo.DeleteMaintenanceRuns(profileID); err != nil {
		return DeleteResult{}, err
	}
	retainedNodeIDs, err := repo.RetainedNodeIDs(profileID)
	if err != nil {
		return DeleteResult{}, err
	}
	if err := repo.DeleteRetainedNodes(profileID); err != nil {
		return DeleteResult{}, err
	}
	affected, err := repo.DeleteProfile(profileID)
	if err != nil {
		return DeleteResult{}, err
	}
	if affected == 0 {
		return DeleteResult{}, ErrProfileNotFound
	}
	deletedFingerprints, err := repo.CleanupNodesWithoutReferences(retainedNodeIDs)
	if err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{DeletedFingerprints: deletedFingerprints}, nil
}
