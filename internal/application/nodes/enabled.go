package nodes

import "context"

type SetEnabledResult struct {
	Updated            bool
	RuntimeFingerprint string
}

func SetEnabled(ctx context.Context, repo Repository, nodeID string, enabled bool) (SetEnabledResult, error) {
	runtimeFingerprint := ""
	if !enabled {
		node, found, err := repo.Load(ctx, nodeID)
		if err != nil {
			return SetEnabledResult{}, err
		}
		if !found {
			return SetEnabledResult{}, ErrNodeNotFound
		}
		runtimeFingerprint = RuntimeFingerprint(node)
	}
	updated, err := repo.SetEnabled(ctx, nodeID, enabled)
	if err != nil {
		return SetEnabledResult{}, err
	}
	if !updated {
		return SetEnabledResult{}, ErrNodeNotFound
	}
	return SetEnabledResult{Updated: true, RuntimeFingerprint: runtimeFingerprint}, nil
}
