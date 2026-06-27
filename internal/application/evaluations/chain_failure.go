package evaluations

type ChainFailureOutcome struct {
	State                        string
	LastError                    string
	SwitchReason                 string
	ClearCurrentPath             bool
	IncrementCurrentPathCounters bool
}

func PlanChainInvalidConfig(lastError, switchReason string) ChainFailureOutcome {
	return ChainFailureOutcome{
		State:            ProfileStateInvalidConfig,
		LastError:        lastError,
		SwitchReason:     switchReason,
		ClearCurrentPath: true,
	}
}

func PlanChainCandidateFilterError(lastError string) ChainFailureOutcome {
	return ChainFailureOutcome{
		State:            ProfileStateFailed,
		LastError:        lastError,
		SwitchReason:     SwitchReasonCandidateFilterError,
		ClearCurrentPath: true,
	}
}

func PlanChainNoFrontCandidate(retainCurrentPath bool) ChainFailureOutcome {
	return planChainNoCandidate("no front node candidates", retainCurrentPath)
}

func PlanChainNoPathCandidate(retainCurrentPath bool) ChainFailureOutcome {
	return planChainNoCandidate("no chain path candidates", retainCurrentPath)
}

func PlanChainAllCandidatesFailed(currentPathExists bool, lastError, defaultLastError string) ChainFailureOutcome {
	if lastError == "" {
		lastError = defaultLastError
	}
	if currentPathExists {
		return ChainFailureOutcome{
			State:                        ProfileStateDegraded,
			LastError:                    lastError,
			SwitchReason:                 SwitchReasonCurrentPathReusedAfterFailure,
			IncrementCurrentPathCounters: true,
		}
	}
	return ChainFailureOutcome{
		State:            ProfileStateFailed,
		LastError:        lastError,
		SwitchReason:     SwitchReasonAllCandidatesFailed,
		ClearCurrentPath: true,
	}
}

func PlanChainMissingExitNode(lastError string, retainCurrentPath bool) ChainFailureOutcome {
	if retainCurrentPath {
		return ChainFailureOutcome{
			State:                        ProfileStateDegraded,
			LastError:                    lastError,
			SwitchReason:                 SwitchReasonCurrentPathReusedAfterFailure,
			IncrementCurrentPathCounters: true,
		}
	}
	return PlanChainInvalidConfig(lastError, SwitchReasonMissingExitNode)
}

func planChainNoCandidate(lastError string, retainCurrentPath bool) ChainFailureOutcome {
	if retainCurrentPath {
		return ChainFailureOutcome{
			State:                        ProfileStateDegraded,
			LastError:                    lastError,
			SwitchReason:                 SwitchReasonCurrentPathReusedAfterFailure,
			IncrementCurrentPathCounters: true,
		}
	}
	return ChainFailureOutcome{
		State:            ProfileStateNoCandidate,
		LastError:        lastError,
		SwitchReason:     SwitchReasonNoCandidate,
		ClearCurrentPath: true,
	}
}
