package maintenance

import "testing"

func TestNextBatchClaimLimitsPrioritizesMutatingRefreshBeforeEvaluations(t *testing.T) {
	limits := NextBatchClaimLimits(ClaimConcurrency{
		SubscriptionRefresh: 2,
		NodeObservation:     3,
		ProfileEvaluation:   4,
		GeoIPUpdate:         5,
	})

	want := []ClaimLimit{
		{RunTypeSubscriptionRefresh, 2},
		{RunTypeNodeObservation, 3},
		{RunTypeProfileEvaluation, 4},
		{RunTypeGeoIPUpdate, 5},
		{RunTypeLogCleanup, 1},
	}
	if len(limits) != len(want) {
		t.Fatalf("limits len = %d, want %d", len(limits), len(want))
	}
	for i := range want {
		if limits[i] != want[i] {
			t.Fatalf("limits[%d] = %#v, want %#v", i, limits[i], want[i])
		}
	}
}

func TestNextBatchClaimLimitsDefaultsNonPositiveLimitsToOne(t *testing.T) {
	limits := NextBatchClaimLimits(ClaimConcurrency{})

	for _, limit := range limits {
		if limit.Limit != 1 {
			t.Fatalf("%s limit = %d, want 1", limit.RunType, limit.Limit)
		}
	}
}
