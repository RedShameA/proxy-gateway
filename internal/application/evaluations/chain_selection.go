package evaluations

import "math"

type ChainSelectionInput struct {
	CurrentFrontNodeID           string
	CurrentExitNodeID            string
	CurrentDurationMS            int64
	BestFrontNodeID              string
	BestExitNodeID               string
	BestDurationMS               int64
	RelativeImprovementThreshold float64
	AbsoluteLatencyImprovementMS int
	ForceSwitch                  bool
}

type ChainSelection struct {
	SelectedFrontNodeID string
	SelectedExitNodeID  string
	SelectedDurationMS  int64
	State               string
	SwitchReason        string
	FailedCount         int
	MissedSuccessCycles int
}

func SelectChainPath(input ChainSelectionInput) ChainSelection {
	if input.CurrentFrontNodeID == "" || input.CurrentExitNodeID == "" || (input.BestFrontNodeID == input.CurrentFrontNodeID && input.BestExitNodeID == input.CurrentExitNodeID) || input.ForceSwitch {
		reason := "initial_selection"
		if input.ForceSwitch && input.CurrentFrontNodeID != "" && input.CurrentExitNodeID != "" && (input.BestFrontNodeID != input.CurrentFrontNodeID || input.BestExitNodeID != input.CurrentExitNodeID) {
			reason = "force_switch"
		} else if input.BestFrontNodeID == input.CurrentFrontNodeID && input.BestExitNodeID == input.CurrentExitNodeID {
			reason = "current_path_still_best"
		}
		return ChainSelection{
			SelectedFrontNodeID: input.BestFrontNodeID,
			SelectedExitNodeID:  input.BestExitNodeID,
			SelectedDurationMS:  input.BestDurationMS,
			State:               "ready",
			SwitchReason:        reason,
		}
	}
	if input.CurrentDurationMS == int64(math.MaxInt64) {
		return ChainSelection{
			SelectedFrontNodeID: input.BestFrontNodeID,
			SelectedExitNodeID:  input.BestExitNodeID,
			SelectedDurationMS:  input.BestDurationMS,
			State:               "ready",
			SwitchReason:        "current_path_failed_switch",
		}
	}
	if clearlyBetter(input.BestDurationMS, input.CurrentDurationMS, input.RelativeImprovementThreshold, input.AbsoluteLatencyImprovementMS) {
		return ChainSelection{
			SelectedFrontNodeID: input.BestFrontNodeID,
			SelectedExitNodeID:  input.BestExitNodeID,
			SelectedDurationMS:  input.BestDurationMS,
			State:               "ready",
			SwitchReason:        "candidate_clearly_better",
		}
	}
	return ChainSelection{
		SelectedFrontNodeID: input.CurrentFrontNodeID,
		SelectedExitNodeID:  input.CurrentExitNodeID,
		SelectedDurationMS:  input.CurrentDurationMS,
		State:               "ready",
		SwitchReason:        "candidate_not_clearly_better",
	}
}
