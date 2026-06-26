package subscriptions

type RefreshSnapshotRepository interface {
	ExistingSourceNodeIDs(subscriptionID string) ([]string, error)
	DeleteSubscriptionNodeSource(nodeID, subscriptionID string) error
	RetainStickyProfilesForRemovedNode(nodeID string) ([]StickyProfileEvaluationRef, error)
	CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error)
	RetainedStickyProfilesForRefresh() ([]StickyProfileEvaluationRef, error)
}

type RefreshSnapshotResult struct {
	DeletedFingerprints      []string
	StickyProfilesToEvaluate []StickyProfileEvaluationRef
}

func PruneRefreshSnapshot(repo RefreshSnapshotRepository, subscriptionID string, currentNodeIDs []string) (RefreshSnapshotResult, error) {
	existingNodeIDs, err := repo.ExistingSourceNodeIDs(subscriptionID)
	if err != nil {
		return RefreshSnapshotResult{}, err
	}
	current := map[string]struct{}{}
	for _, nodeID := range currentNodeIDs {
		current[nodeID] = struct{}{}
	}
	var staleNodeIDs []string
	var stickyProfilesToEvaluate []StickyProfileEvaluationRef
	for _, nodeID := range existingNodeIDs {
		if _, ok := current[nodeID]; ok {
			continue
		}
		staleNodeIDs = append(staleNodeIDs, nodeID)
		if err := repo.DeleteSubscriptionNodeSource(nodeID, subscriptionID); err != nil {
			return RefreshSnapshotResult{}, err
		}
		refs, err := repo.RetainStickyProfilesForRemovedNode(nodeID)
		if err != nil {
			return RefreshSnapshotResult{}, err
		}
		stickyProfilesToEvaluate = append(stickyProfilesToEvaluate, refs...)
	}
	deletedFingerprints, err := repo.CleanupNodesWithoutReferences(staleNodeIDs)
	if err != nil {
		return RefreshSnapshotResult{}, err
	}
	retainedSticky, err := repo.RetainedStickyProfilesForRefresh()
	if err != nil {
		return RefreshSnapshotResult{}, err
	}
	stickyProfilesToEvaluate = append(stickyProfilesToEvaluate, retainedSticky...)
	return RefreshSnapshotResult{
		DeletedFingerprints:      deletedFingerprints,
		StickyProfilesToEvaluate: dedupeStickyRefs(stickyProfilesToEvaluate),
	}, nil
}

func dedupeStickyRefs(refs []StickyProfileEvaluationRef) []StickyProfileEvaluationRef {
	if len(refs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]StickyProfileEvaluationRef, 0, len(refs))
	for _, ref := range refs {
		if ref.ID == "" || seen[ref.ID] {
			continue
		}
		seen[ref.ID] = true
		out = append(out, ref)
	}
	return out
}
