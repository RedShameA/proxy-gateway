package subscriptions

import "testing"

func TestBuildRefreshSuccessOutcomeSplitsIgnoredAndSkippedSummaries(t *testing.T) {
	outcome := BuildRefreshSuccessOutcome(RefreshImportResult{
		SubscriptionID: "sub-1",
		ImportedNodes:  3,
		SkippedEntrySummary: []SkippedEntrySummary{
			{Reason: "clash_proxy_group_ignored", Count: 2},
			{Reason: "unsupported_functional_outbound", Count: 1},
			{Reason: "malformed_entry", Count: 4},
		},
		StickyProfilesToEvaluate: []StickyProfileEvaluationRef{
			{ID: "profile-1", Name: "p1", ConfigVersion: 7},
		},
	})

	if outcome.SubscriptionID != "sub-1" || outcome.ImportedCount != 3 || !outcome.EnqueueObservation {
		t.Fatalf("outcome base fields = %#v", outcome)
	}
	if outcome.IgnoredCount != 3 || outcome.SkippedCount != 4 {
		t.Fatalf("ignored/skipped counts = %d/%d, want 3/4", outcome.IgnoredCount, outcome.SkippedCount)
	}
	if len(outcome.IgnoredSummary) != 2 || len(outcome.SkippedSummary) != 1 {
		t.Fatalf("ignored/skipped summaries = %#v / %#v", outcome.IgnoredSummary, outcome.SkippedSummary)
	}
	if len(outcome.StickyProfilesToEvaluate) != 1 || outcome.StickyProfilesToEvaluate[0].ID != "profile-1" {
		t.Fatalf("sticky refs = %#v", outcome.StickyProfilesToEvaluate)
	}
}
