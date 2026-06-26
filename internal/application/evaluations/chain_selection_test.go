package evaluations

import "testing"

func TestSelectChainPathKeepsCurrentWhenCandidateIsNotClearlyBetter(t *testing.T) {
	selection := SelectChainPath(ChainSelectionInput{
		CurrentFrontNodeID:           "front-current",
		CurrentExitNodeID:            "exit-current",
		CurrentDurationMS:            100,
		BestFrontNodeID:              "front-best",
		BestExitNodeID:               "exit-best",
		BestDurationMS:               85,
		RelativeImprovementThreshold: 0.20,
		AbsoluteLatencyImprovementMS: 30,
	})

	if selection.SelectedFrontNodeID != "front-current" || selection.SelectedExitNodeID != "exit-current" {
		t.Fatalf("selection = %#v", selection)
	}
	if selection.SelectedDurationMS != 100 || selection.SwitchReason != "candidate_not_clearly_better" || selection.State != "ready" {
		t.Fatalf("selection state = %#v", selection)
	}
}

func TestSelectChainPathSwitchesWhenCurrentPathProbeFails(t *testing.T) {
	selection := SelectChainPath(ChainSelectionInput{
		CurrentFrontNodeID: "front-current",
		CurrentExitNodeID:  "exit-current",
		CurrentDurationMS:  1<<63 - 1,
		BestFrontNodeID:    "front-best",
		BestExitNodeID:     "exit-best",
		BestDurationMS:     80,
	})

	if selection.SelectedFrontNodeID != "front-best" || selection.SelectedExitNodeID != "exit-best" {
		t.Fatalf("selection = %#v", selection)
	}
	if selection.SelectedDurationMS != 80 || selection.SwitchReason != "current_path_failed_switch" || selection.State != "ready" {
		t.Fatalf("selection state = %#v", selection)
	}
}
