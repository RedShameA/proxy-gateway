package evaluations

import "math"

type FastestSelectionInput struct {
	CurrentNodeID                string
	CurrentDurationMS            int64
	BestNodeID                   string
	BestDurationMS               int64
	RelativeImprovementThreshold float64
	AbsoluteLatencyImprovementMS int
	ForceSwitch                  bool
}

type FastestSelection struct {
	SelectedNodeID      string
	SelectedDurationMS  int64
	State               string
	SwitchReason        string
	FailedCount         int
	MissedSuccessCycles int
}

func SelectFastestPath(input FastestSelectionInput) FastestSelection {
	if input.CurrentNodeID == "" || input.BestNodeID == input.CurrentNodeID || input.ForceSwitch {
		reason := SwitchReasonInitialSelection
		if input.ForceSwitch && input.CurrentNodeID != "" && input.BestNodeID != input.CurrentNodeID {
			reason = SwitchReasonForceSwitch
		} else if input.BestNodeID == input.CurrentNodeID {
			reason = SwitchReasonCurrentPathStillBest
		}
		return FastestSelection{
			SelectedNodeID:     input.BestNodeID,
			SelectedDurationMS: input.BestDurationMS,
			State:              ProfileStateReady,
			SwitchReason:       reason,
		}
	}
	if input.CurrentDurationMS == int64(math.MaxInt64) {
		return FastestSelection{
			SelectedNodeID:     input.BestNodeID,
			SelectedDurationMS: input.BestDurationMS,
			State:              ProfileStateReady,
			SwitchReason:       SwitchReasonCurrentPathFailedSwitch,
		}
	}
	if clearlyBetter(input.BestDurationMS, input.CurrentDurationMS, input.RelativeImprovementThreshold, input.AbsoluteLatencyImprovementMS) {
		return FastestSelection{
			SelectedNodeID:     input.BestNodeID,
			SelectedDurationMS: input.BestDurationMS,
			State:              ProfileStateReady,
			SwitchReason:       SwitchReasonCandidateClearlyBetter,
		}
	}
	return FastestSelection{
		SelectedNodeID:     input.CurrentNodeID,
		SelectedDurationMS: input.CurrentDurationMS,
		State:              ProfileStateReady,
		SwitchReason:       SwitchReasonCandidateNotClearlyBetter,
	}
}

func clearlyBetter(candidateDuration, currentDuration int64, relativeThreshold float64, absoluteThresholdMS int) bool {
	if candidateDuration >= currentDuration {
		return false
	}
	improvement := currentDuration - candidateDuration
	if relativeThreshold <= 0 && absoluteThresholdMS <= 0 {
		return true
	}
	if relativeThreshold > 0 {
		relativeThresholdMS := int64(math.Ceil(float64(currentDuration) * relativeThreshold))
		if improvement >= relativeThresholdMS {
			return true
		}
	}
	if absoluteThresholdMS > 0 && improvement >= int64(absoluteThresholdMS) {
		return true
	}
	return false
}
