package overview

import "context"

type ResourceCounts struct {
	Subscriptions int
	Nodes         int
	UsableNodes   int
	Profiles      int
	Credentials   int
}

type Repository interface {
	LoadResourceCounts(ctx context.Context) (ResourceCounts, error)
	LoadProfileStateCounts(ctx context.Context) (map[string]int, error)
}
