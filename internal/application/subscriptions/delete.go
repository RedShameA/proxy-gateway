package subscriptions

import "errors"

var ErrSubscriptionNotFound = errors.New("subscription not found")

type DeleteRepository interface {
	DeleteSubscription(subscriptionID string) (int64, error)
	NodeIDsForSource(subscriptionID, sourceType string) ([]string, error)
	DeleteSubscriptionNodeSources(subscriptionID string) error
	InvalidateProfilesForDeletedSubscription(subscriptionID string) error
	CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error)
}

type DeleteResult struct {
	DeletedFingerprints []string
}

func DeleteSubscriptionSource(repo DeleteRepository, subscriptionID string) (DeleteResult, error) {
	affected, err := repo.DeleteSubscription(subscriptionID)
	if err != nil {
		return DeleteResult{}, err
	}
	if affected == 0 {
		return DeleteResult{}, ErrSubscriptionNotFound
	}
	nodeIDs, err := repo.NodeIDsForSource(subscriptionID, "subscription")
	if err != nil {
		return DeleteResult{}, err
	}
	if err := repo.DeleteSubscriptionNodeSources(subscriptionID); err != nil {
		return DeleteResult{}, err
	}
	if err := repo.InvalidateProfilesForDeletedSubscription(subscriptionID); err != nil {
		return DeleteResult{}, err
	}
	deletedFingerprints, err := repo.CleanupNodesWithoutReferences(nodeIDs)
	if err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{DeletedFingerprints: deletedFingerprints}, nil
}
