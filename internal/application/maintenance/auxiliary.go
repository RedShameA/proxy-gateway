package maintenance

import "context"

type NodeObservationScheduleTarget struct {
	ID   string
	Name string
}

type ProfileEvaluationScheduleTarget struct {
	ID                            string
	Name                          string
	ProfileType                   string
	LastEvaluatedAt               int64
	AutoEvaluationEnabled         bool
	AutoEvaluationIntervalSeconds int
	ConfigVersion                 int64
}

type SubscriptionRefreshScheduleTarget struct {
	ID                         string
	Name                       string
	UpdatedAt                  int64
	AutoRefreshEnabled         bool
	AutoRefreshIntervalSeconds int
}

type WaitingObservationProfile struct {
	ID            string
	Name          string
	ConfigVersion int64
}

type AuxiliaryRepository interface {
	ListNodeObservationScheduleTargets(ctx context.Context) ([]NodeObservationScheduleTarget, error)
	ListSubscriptionNodeObservationTargets(ctx context.Context, subscriptionID string) ([]NodeObservationScheduleTarget, error)
	ListProfileEvaluationScheduleTargets(ctx context.Context) ([]ProfileEvaluationScheduleTarget, error)
	ListProfilesWaitingForObservation(ctx context.Context) ([]WaitingObservationProfile, error)
	ListSubscriptionRefreshScheduleTargets(ctx context.Context) ([]SubscriptionRefreshScheduleTarget, error)
	HasRecentRun(ctx context.Context, runType string, createdAfterMillis int64) (bool, error)
	HasUnfinishedCurrentNodeObservedEvaluation(ctx context.Context, profileID string) (bool, error)
	DeleteHistoryBefore(ctx context.Context, cutoffMillis int64, keepRunID string) (int64, error)
}
