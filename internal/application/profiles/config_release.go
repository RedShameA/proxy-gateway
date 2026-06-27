package profiles

import "context"

type ConfigReleaseTx interface {
	ProfileConfigRepository() ConfigUpdater
	ReleaseRetainedProfileNodesExcept(profileID string, keepNodeIDs []string) ([]string, error)
}

type ConfigReleaseTxRunner interface {
	WithProfileConfigReleaseTx(ctx context.Context, fn func(ConfigReleaseTx) error) error
}

func UpdateConfigWithRelease(runner ConfigReleaseTxRunner) func(context.Context, string, ConfigRecord, ConfigUpdateOptions) ([]string, error) {
	return func(ctx context.Context, profileID string, cfg ConfigRecord, options ConfigUpdateOptions) ([]string, error) {
		var deletedFingerprints []string
		err := runner.WithProfileConfigReleaseTx(ctx, func(tx ConfigReleaseTx) error {
			if err := tx.ProfileConfigRepository().UpdateConfig(ctx, cfg, options); err != nil {
				return err
			}
			var releaseErr error
			deletedFingerprints, releaseErr = tx.ReleaseRetainedProfileNodesExcept(profileID, nil)
			return releaseErr
		})
		return deletedFingerprints, err
	}
}
