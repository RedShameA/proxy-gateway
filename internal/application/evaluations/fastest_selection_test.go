package evaluations

import "testing"

func TestSelectFastestPathKeepsCurrentWhenCandidateIsNotClearlyBetter(t *testing.T) {
	selection := SelectFastestPath(FastestSelectionInput{
		CurrentNodeID:                "node-current",
		CurrentDurationMS:            100,
		BestNodeID:                   "node-challenger",
		BestDurationMS:               85,
		RelativeImprovementThreshold: 0.20,
		AbsoluteLatencyImprovementMS: 30,
	})

	if selection.SelectedNodeID != "node-current" || selection.SelectedDurationMS != 100 {
		t.Fatalf("selection = %#v", selection)
	}
	if selection.State != "ready" || selection.SwitchReason != "candidate_not_clearly_better" {
		t.Fatalf("selection state = %#v", selection)
	}
}

func TestSelectFastestPathSwitchesImmediatelyWhenCurrentPathProbeFails(t *testing.T) {
	selection := SelectFastestPath(FastestSelectionInput{
		CurrentNodeID:     "node-current",
		CurrentDurationMS: 1<<63 - 1,
		BestNodeID:        "node-failover",
		BestDurationMS:    80,
	})

	if selection.SelectedNodeID != "node-failover" || selection.SelectedDurationMS != 80 {
		t.Fatalf("selection = %#v", selection)
	}
	if selection.State != "ready" || selection.SwitchReason != "current_path_failed_switch" {
		t.Fatalf("selection state = %#v", selection)
	}
}
