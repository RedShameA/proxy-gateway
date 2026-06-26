package evaluations

import "math"

type FastestDetailsInput struct {
	TestURL           string
	CandidateCount    int
	FailureCount      int
	BestNodeID        string
	BestDurationMS    int64
	CurrentNodeID     string
	CurrentDurationMS int64
	SelectedNodeID    string
	SwitchReason      string
}

func BuildFastestDetails(input FastestDetailsInput) map[string]any {
	details := map[string]any{
		"test_url":         input.TestURL,
		"candidate_count":  input.CandidateCount,
		"failure_count":    input.FailureCount,
		"best_node_id":     input.BestNodeID,
		"current_node_id":  input.CurrentNodeID,
		"selected_node_id": input.SelectedNodeID,
		"switch_reason":    input.SwitchReason,
	}
	if input.BestDurationMS != int64(math.MaxInt64) {
		details["best_latency_ms"] = input.BestDurationMS
	}
	if input.CurrentDurationMS != int64(math.MaxInt64) {
		details["current_latency_ms"] = input.CurrentDurationMS
	}
	return details
}
