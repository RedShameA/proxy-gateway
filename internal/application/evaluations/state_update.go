package evaluations

func RunningStateUpdate(startedAt int64) StateUpdate {
	return StateUpdate{
		State:                   stringPtr(ProfileStateRunning),
		LastError:               stringPtr(""),
		LastEvaluationStartedAt: int64Ptr(startedAt),
	}
}

func (outcome FastestFailureOutcome) StateUpdate(finishedAt int64) StateUpdate {
	update := StateUpdate{
		State:                        stringPtr(outcome.State),
		LastError:                    stringPtr(outcome.LastError),
		IncrementCurrentPathCounters: outcome.IncrementCurrentPathCounters,
		SwitchReason:                 stringPtr(outcome.SwitchReason),
		LastEvaluatedAt:              int64Ptr(finishedAt),
	}
	if outcome.ClearCurrentNode {
		update.CurrentNodeID = stringPtr("")
	}
	return update
}

func (outcome ChainFailureOutcome) StateUpdate(finishedAt int64) StateUpdate {
	update := StateUpdate{
		State:                        stringPtr(outcome.State),
		LastError:                    stringPtr(outcome.LastError),
		IncrementCurrentPathCounters: outcome.IncrementCurrentPathCounters,
		SwitchReason:                 stringPtr(outcome.SwitchReason),
		LastEvaluatedAt:              int64Ptr(finishedAt),
	}
	if outcome.ClearCurrentPath {
		update.CurrentNodeID = stringPtr("")
		update.CurrentExitNodeID = stringPtr("")
	}
	return update
}

func (selection FastestSelection) StateUpdate(detailsJSON string, finishedAt int64) StateUpdate {
	return StateUpdate{
		State:                          stringPtr(selection.State),
		CurrentNodeID:                  stringPtr(selection.SelectedNodeID),
		LastError:                      stringPtr(""),
		CurrentPathLatencyMS:           int64Ptr(selection.SelectedDurationMS),
		CurrentPathFailedEvaluations:   intPtr(selection.FailedCount),
		CurrentPathMissedSuccessCycles: intPtr(selection.MissedSuccessCycles),
		SwitchReason:                   stringPtr(selection.SwitchReason),
		LastEvaluationDetailsJSON:      stringPtr(detailsJSON),
		LastEvaluatedAt:                int64Ptr(finishedAt),
	}
}

func (selection ChainSelection) StateUpdate(detailsJSON string, finishedAt int64) StateUpdate {
	return StateUpdate{
		State:                          stringPtr(selection.State),
		CurrentNodeID:                  stringPtr(selection.SelectedFrontNodeID),
		CurrentExitNodeID:              stringPtr(selection.SelectedExitNodeID),
		LastError:                      stringPtr(""),
		CurrentPathLatencyMS:           int64Ptr(selection.SelectedDurationMS),
		CurrentPathFailedEvaluations:   intPtr(selection.FailedCount),
		CurrentPathMissedSuccessCycles: intPtr(selection.MissedSuccessCycles),
		SwitchReason:                   stringPtr(selection.SwitchReason),
		LastEvaluationDetailsJSON:      stringPtr(detailsJSON),
		LastEvaluatedAt:                int64Ptr(finishedAt),
	}
}

func FastestSelectedNodeRemovedUpdate(detailsJSON string, finishedAt int64) StateUpdate {
	return StateUpdate{
		State:                        stringPtr("waiting_observation"),
		CurrentNodeID:                stringPtr(""),
		LastError:                    stringPtr("selected node no longer exists"),
		CurrentPathLatencyMS:         int64Ptr(0),
		IncrementCurrentPathCounters: true,
		SwitchReason:                 stringPtr(SwitchReasonSelectedNodeRemoved),
		LastEvaluationDetailsJSON:    stringPtr(detailsJSON),
		LastEvaluatedAt:              int64Ptr(finishedAt),
	}
}

func ChainSelectedPathRemovedUpdate(detailsJSON string, finishedAt int64) StateUpdate {
	return StateUpdate{
		State:                        stringPtr("waiting_observation"),
		CurrentNodeID:                stringPtr(""),
		CurrentExitNodeID:            stringPtr(""),
		LastError:                    stringPtr("selected chain path no longer exists"),
		CurrentPathLatencyMS:         int64Ptr(0),
		IncrementCurrentPathCounters: true,
		SwitchReason:                 stringPtr(SwitchReasonSelectedNodeRemoved),
		LastEvaluationDetailsJSON:    stringPtr(detailsJSON),
		LastEvaluatedAt:              int64Ptr(finishedAt),
	}
}

func stringPtr(value string) *string {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
