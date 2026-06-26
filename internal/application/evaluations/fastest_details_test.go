package evaluations

import "testing"

func TestBuildFastestDetailsIncludesCandidateFailureSelectionAndLatency(t *testing.T) {
	details := BuildFastestDetails(FastestDetailsInput{
		TestURL:           "https://example.test/probe",
		CandidateCount:    3,
		FailureCount:      1,
		BestNodeID:        "node-best",
		BestDurationMS:    80,
		CurrentNodeID:     "node-current",
		CurrentDurationMS: 120,
		SelectedNodeID:    "node-best",
		SwitchReason:      "candidate_clearly_better",
	})

	if details["test_url"] != "https://example.test/probe" || details["candidate_count"] != 3 || details["failure_count"] != 1 {
		t.Fatalf("counts = %#v", details)
	}
	if details["best_node_id"] != "node-best" || details["current_node_id"] != "node-current" || details["selected_node_id"] != "node-best" {
		t.Fatalf("nodes = %#v", details)
	}
	if details["best_latency_ms"] != int64(80) || details["current_latency_ms"] != int64(120) {
		t.Fatalf("latencies = %#v", details)
	}
	if details["switch_reason"] != "candidate_clearly_better" {
		t.Fatalf("switch_reason = %#v", details["switch_reason"])
	}
}
