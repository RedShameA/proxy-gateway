package uow

import (
	"context"

	appnodes "proxygateway/internal/application/nodes"
	appprofiles "proxygateway/internal/application/profiles"
	appsubscriptions "proxygateway/internal/application/subscriptions"
)

type Runner interface {
	WithTx(ctx context.Context, fn func(Tx) error) error
}

type Tx interface {
	NodeUpsertRepository() appnodes.UpsertRepository
	NodeManualUpdateRepository() appnodes.ManualUpdateRepository
	NodeDeleteRepository() appnodes.DeleteRepository
	SubscriptionImportRepository() appsubscriptions.ImportRepository
	SubscriptionSourceRepository(nowMillis int64) appsubscriptions.SourceRepository
	ProfileDeleteRepository() appprofiles.DeleteRepository
	ProfileConfigRepository() appprofiles.ConfigUpdater
	ReleaseRetainedProfileNodesExcept(profileID string, keepNodeIDs []string) ([]string, error)
}
