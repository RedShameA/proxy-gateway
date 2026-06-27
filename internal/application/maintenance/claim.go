package maintenance

type ClaimConcurrency struct {
	SubscriptionRefresh int
	NodeObservation     int
	ProfileEvaluation   int
	GeoIPUpdate         int
}

type ClaimLimit struct {
	RunType string
	Limit   int
}

func NextBatchClaimLimits(settings ClaimConcurrency) []ClaimLimit {
	return []ClaimLimit{
		{RunType: RunTypeSubscriptionRefresh, Limit: positiveLimit(settings.SubscriptionRefresh)},
		{RunType: RunTypeNodeObservation, Limit: positiveLimit(settings.NodeObservation)},
		{RunType: RunTypeProfileEvaluation, Limit: positiveLimit(settings.ProfileEvaluation)},
		{RunType: RunTypeGeoIPUpdate, Limit: positiveLimit(settings.GeoIPUpdate)},
		{RunType: RunTypeLogCleanup, Limit: 1},
	}
}

func positiveLimit(limit int) int {
	if limit <= 0 {
		return 1
	}
	return limit
}
