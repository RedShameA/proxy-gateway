package evaluations

import "testing"

func TestPlanFastestNoCandidateDegradesWhenStickyCurrentPathCanBeRetained(t *testing.T) {
	outcome := PlanFastestNoCandidate("no candidate nodes", true)

	if outcome.State != "degraded" || outcome.SwitchReason != "current_path_reused_after_failure" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if !outcome.IncrementCurrentPathCounters || outcome.ClearCurrentNode {
		t.Fatalf("outcome counters/current = %#v", outcome)
	}
}

func TestPlanFastestAllCandidatesFailedFailsWhenNoCurrentPathExists(t *testing.T) {
	outcome := PlanFastestAllCandidatesFailed("", "dial failed")

	if outcome.State != "failed" || outcome.SwitchReason != "all_candidates_failed" || outcome.LastError != "dial failed" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if !outcome.ClearCurrentNode || outcome.IncrementCurrentPathCounters {
		t.Fatalf("outcome counters/current = %#v", outcome)
	}
}
