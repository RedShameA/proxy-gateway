package evaluations

import "testing"

func TestSummarizeChainProbeResultsTracksBestCurrentPairAndFailures(t *testing.T) {
	summary := SummarizeChainProbeResults("front-current", "exit-current", []ChainProbeResult{
		{FrontNodeID: "front-current", ExitNodeID: "exit-current", DurationMS: 120, OK: true},
		{FrontNodeID: "front-best", ExitNodeID: "exit-best", DurationMS: 80, OK: true},
		{FrontNodeID: "front-failed", ExitNodeID: "exit-best", Error: "chain failed"},
	})

	if summary.BestFrontNodeID != "front-best" || summary.BestExitNodeID != "exit-best" || summary.BestDurationMS != 80 {
		t.Fatalf("best = %#v", summary)
	}
	if summary.CurrentDurationMS != 120 {
		t.Fatalf("CurrentDurationMS = %d", summary.CurrentDurationMS)
	}
	if summary.FailureCount != 1 || summary.LastFailure != "chain failed" {
		t.Fatalf("failures = %d/%q", summary.FailureCount, summary.LastFailure)
	}
}
