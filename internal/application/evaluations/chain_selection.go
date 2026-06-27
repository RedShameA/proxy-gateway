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
		reason := SwitchReasonInitialSelection
		if input.ForceSwitch && input.CurrentFrontNodeID != "" && input.CurrentExitNodeID != "" && (input.BestFrontNodeID != input.CurrentFrontNodeID || input.BestExitNodeID != input.CurrentExitNodeID) {
			reason = SwitchReasonForceSwitch
		} else if input.BestFrontNodeID == input.CurrentFrontNodeID && input.BestExitNodeID == input.CurrentExitNodeID {
			reason = SwitchReasonCurrentPathStillBest
		}
		return ChainSelection{
			SelectedFrontNodeID: input.BestFrontNodeID,
			SelectedExitNodeID:  input.BestExitNodeID,
			SelectedDurationMS:  input.BestDurationMS,
			State:               ProfileStateReady,
			SwitchReason:        reason,
		}
	}
	if input.CurrentDurationMS == int64(math.MaxInt64) {
		return ChainSelection{
			SelectedFrontNodeID: input.BestFrontNodeID,
			SelectedExitNodeID:  input.BestExitNodeID,
			SelectedDurationMS:  input.BestDurationMS,
			State:               ProfileStateReady,
			SwitchReason:        SwitchReasonCurrentPathFailedSwitch,
		}
	}
	if clearlyBetter(input.BestDurationMS, input.CurrentDurationMS, input.RelativeImprovementThreshold, input.AbsoluteLatencyImprovementMS) {
		return ChainSelection{
			SelectedFrontNodeID: input.BestFrontNodeID,
			SelectedExitNodeID:  input.BestExitNodeID,
			SelectedDurationMS:  input.BestDurationMS,
			State:               ProfileStateReady,
			SwitchReason:        SwitchReasonCandidateClearlyBetter,
		}
	}
	return ChainSelection{
		SelectedFrontNodeID: input.CurrentFrontNodeID,
		SelectedExitNodeID:  input.CurrentExitNodeID,
		SelectedDurationMS:  input.CurrentDurationMS,
		State:               ProfileStateReady,
		SwitchReason:        SwitchReasonCandidateNotClearlyBetter,
	}
}
