package subscriptions

import (
	"testing"

	appmaintenance "proxygateway/internal/application/maintenance"
)

func TestBuildRefreshSuccessOutcomeSplitsIgnoredAndSkippedSummaries(t *testing.T) {
	outcome := BuildRefreshSuccessOutcome(RefreshImportResult{
		SubscriptionID: "sub-1",
		ImportedNodes:  3,
		SkippedEntrySummary: []SkippedEntrySummary{
			{Reason: SkippedReasonClashProxyGroupIgnored, Count: 2},
			{Reason: SkippedReasonUnsupportedFunctionalOutbound, Count: 1},
			{Reason: SkippedReasonMalformedEntry, Count: 4},
		},
		StickyProfilesToEvaluate: []StickyProfileEvaluationRef{
			{ID: "profile-1", Name: "p1", ConfigVersion: 7},
		},
	})

	if outcome.SubscriptionID != "sub-1" || outcome.Result != appmaintenance.ResultSuccess || outcome.ReasonCode != appmaintenance.ReasonCompleted || outcome.ImportedCount != 3 || !outcome.EnqueueObservation {
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

func TestBuildRefreshSuccessOutcomeWarnsWhenNoNodesImported(t *testing.T) {
	outcome := BuildRefreshSuccessOutcome(RefreshImportResult{
		SubscriptionID: "sub-empty",
		ImportedNodes:  0,
		SkippedEntrySummary: []SkippedEntrySummary{
			{Reason: SkippedReasonMalformedEntry, Count: 2},
		},
	})

	if outcome.Result != appmaintenance.ResultWarning || outcome.ReasonCode != appmaintenance.ReasonNoImportableNodes {
		t.Fatalf("outcome status = %#v, want warning no_importable_nodes", outcome)
	}
	if outcome.ImportedCount != 0 || outcome.SkippedCount != 2 {
		t.Fatalf("outcome counts = %#v, want imported 0 skipped 2", outcome)
	}
}

func TestRefreshSuccessOutcomeBuildsMaintenanceDetail(t *testing.T) {
	outcome := BuildRefreshSuccessOutcome(RefreshImportResult{
		SubscriptionID: "sub-1",
		ImportedNodes:  3,
		SkippedEntrySummary: []SkippedEntrySummary{
			{Reason: SkippedReasonClashProxyGroupIgnored, Count: 2},
			{Reason: SkippedReasonMalformedEntry, Count: 1},
		},
	})
	base := map[string]any{"existing": true}

	detail := outcome.MaintenanceDetail(base)

	if detail["subscription_id"] != "sub-1" || detail["imported_count"] != 3 || detail["imported"] != 3 {
		t.Fatalf("detail import fields = %#v", detail)
	}
	if detail["retained_count"] != 3 || detail["added_count"] != 0 || detail["updated_count"] != 0 || detail["removed_count"] != 0 {
		t.Fatalf("detail mutation counts = %#v", detail)
	}
	if detail["ignored_count"] != 2 || detail["skipped_count"] != 1 {
		t.Fatalf("detail skip counts = %#v", detail)
	}
	if base["subscription_id"] != nil {
		t.Fatalf("base detail was mutated: %#v", base)
	}
}

func TestRefreshFailureOutcomeBuildsMaintenanceDetail(t *testing.T) {
	outcome := BuildRefreshFetchFailure("sub-1")

	detail := outcome.MaintenanceDetail(nil)

	if detail["subscription_id"] != "sub-1" {
		t.Fatalf("detail = %#v", detail)
	}
}
