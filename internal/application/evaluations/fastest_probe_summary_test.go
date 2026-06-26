package evaluations

import "testing"

func TestSummarizeFastestProbeResultsTracksBestCurrentAndFailures(t *testing.T) {
	summary := SummarizeFastestProbeResults("node-current", []FastestProbeResult{
		{NodeID: "node-current", DurationMS: 120, OK: true},
		{NodeID: "node-best", DurationMS: 80, OK: true},
		{NodeID: "node-failed", Error: "dial failed"},
	})

	if summary.BestNodeID != "node-best" || summary.BestDurationMS != 80 {
		t.Fatalf("best = %q/%d", summary.BestNodeID, summary.BestDurationMS)
	}
	if summary.CurrentDurationMS != 120 {
		t.Fatalf("CurrentDurationMS = %d", summary.CurrentDurationMS)
	}
	if summary.FailureCount != 1 || summary.LastFailure != "dial failed" {
		t.Fatalf("failures = %d/%q", summary.FailureCount, summary.LastFailure)
	}
}
