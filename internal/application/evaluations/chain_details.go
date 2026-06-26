package evaluations

import "math"

type ChainDetailsInput struct {
	TestURL             string
	CandidateCount      int
	FailureCount        int
	BestFrontNodeID     string
	BestExitNodeID      string
	BestDurationMS      int64
	CurrentFrontNodeID  string
	CurrentExitNodeID   string
	CurrentDurationMS   int64
	SelectedFrontNodeID string
	SelectedExitNodeID  string
	SwitchReason        string
}

func BuildChainDetails(input ChainDetailsInput) map[string]any {
	details := map[string]any{
		"test_url":              input.TestURL,
		"candidate_count":       input.CandidateCount,
		"failure_count":         input.FailureCount,
		"best_node_id":          input.BestFrontNodeID,
		"best_exit_node_id":     input.BestExitNodeID,
		"current_node_id":       input.CurrentFrontNodeID,
		"current_exit_node_id":  input.CurrentExitNodeID,
		"selected_node_id":      input.SelectedFrontNodeID,
		"selected_exit_node_id": input.SelectedExitNodeID,
		"exit_node_id":          input.SelectedExitNodeID,
		"switch_reason":         input.SwitchReason,
	}
	if input.BestDurationMS != int64(math.MaxInt64) {
		details["best_latency_ms"] = input.BestDurationMS
	}
	if input.CurrentDurationMS != int64(math.MaxInt64) {
		details["current_latency_ms"] = input.CurrentDurationMS
	}
	return details
}
