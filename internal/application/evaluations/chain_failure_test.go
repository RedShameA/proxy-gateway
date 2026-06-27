package evaluations

import "testing"

func TestPlanChainMissingExitNodeDegradesWhenStickyCurrentPathCanBeRetained(t *testing.T) {
	outcome := PlanChainMissingExitNode("node not found", true)

	if outcome.State != "degraded" || outcome.SwitchReason != "current_path_reused_after_failure" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if !outcome.IncrementCurrentPathCounters || outcome.ClearCurrentPath {
		t.Fatalf("outcome counters/current path = %#v", outcome)
	}
}

func TestPlanChainNoPathCandidateClearsCurrentPathWhenNoRetainedPathExists(t *testing.T) {
	outcome := PlanChainNoPathCandidate(false)

	if outcome.State != "no_candidate" || outcome.LastError != "no chain path candidates" || outcome.SwitchReason != "no_candidate" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if !outcome.ClearCurrentPath || outcome.IncrementCurrentPathCounters {
		t.Fatalf("outcome counters/current path = %#v", outcome)
	}
}

func TestPlanChainAllCandidatesFailedFallsBackToDefaultFailureMessage(t *testing.T) {
	outcome := PlanChainAllCandidatesFailed(false, "", "all chain path candidates failed")

	if outcome.State != "failed" || outcome.LastError != "all chain path candidates failed" || outcome.SwitchReason != "all_candidates_failed" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if !outcome.ClearCurrentPath {
		t.Fatalf("outcome did not clear current path: %#v", outcome)
	}
}
