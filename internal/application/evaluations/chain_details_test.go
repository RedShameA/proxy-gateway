package evaluations

import "testing"

func TestBuildChainDetailsIncludesFrontExitSelectionAndLatency(t *testing.T) {
	details := BuildChainDetails(ChainDetailsInput{
		TestURL:             "https://example.test/probe",
		CandidateCount:      4,
		FailureCount:        1,
		BestFrontNodeID:     "front-best",
		BestExitNodeID:      "exit-best",
		BestDurationMS:      80,
		CurrentFrontNodeID:  "front-current",
		CurrentExitNodeID:   "exit-current",
		CurrentDurationMS:   120,
		SelectedFrontNodeID: "front-best",
		SelectedExitNodeID:  "exit-best",
		SwitchReason:        "candidate_clearly_better",
	})

	if details["candidate_count"] != 4 || details["failure_count"] != 1 || details["test_url"] != "https://example.test/probe" {
		t.Fatalf("counts = %#v", details)
	}
	if details["best_node_id"] != "front-best" || details["best_exit_node_id"] != "exit-best" {
		t.Fatalf("best = %#v", details)
	}
	if details["current_node_id"] != "front-current" || details["current_exit_node_id"] != "exit-current" {
		t.Fatalf("current = %#v", details)
	}
	if details["selected_node_id"] != "front-best" || details["selected_exit_node_id"] != "exit-best" || details["exit_node_id"] != "exit-best" {
		t.Fatalf("selected = %#v", details)
	}
	if details["best_latency_ms"] != int64(80) || details["current_latency_ms"] != int64(120) {
		t.Fatalf("latencies = %#v", details)
	}
}
