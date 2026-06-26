package nodes

type DeleteRepository interface {
	DeleteManualSource(nodeID string) (int64, error)
	CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error)
}

type DeleteResult struct {
	DeletedFingerprints []string
}

func DeleteManualSource(repo DeleteRepository, nodeID string) (DeleteResult, error) {
	affected, err := repo.DeleteManualSource(nodeID)
	if err != nil {
		return DeleteResult{}, err
	}
	if affected == 0 {
		return DeleteResult{}, ErrManualNodeSourceMissing
	}
	deletedFingerprints, err := repo.CleanupNodesWithoutReferences([]string{nodeID})
	if err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{DeletedFingerprints: deletedFingerprints}, nil
}
