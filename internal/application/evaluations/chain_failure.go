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
		State:            "invalid_config",
		LastError:        lastError,
		SwitchReason:     switchReason,
		ClearCurrentPath: true,
	}
}

func PlanChainCandidateFilterError(lastError string) ChainFailureOutcome {
	return ChainFailureOutcome{
		State:            "failed",
		LastError:        lastError,
		SwitchReason:     "candidate_filter_error",
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
			State:                        "degraded",
			LastError:                    lastError,
			SwitchReason:                 "current_path_reused_after_failure",
			IncrementCurrentPathCounters: true,
		}
	}
	return ChainFailureOutcome{
		State:            "failed",
		LastError:        lastError,
		SwitchReason:     "all_candidates_failed",
		ClearCurrentPath: true,
	}
}

func PlanChainMissingExitNode(lastError string, retainCurrentPath bool) ChainFailureOutcome {
	if retainCurrentPath {
		return ChainFailureOutcome{
			State:                        "degraded",
			LastError:                    lastError,
			SwitchReason:                 "current_path_reused_after_failure",
			IncrementCurrentPathCounters: true,
		}
	}
	return PlanChainInvalidConfig(lastError, "missing_exit_node")
}

func planChainNoCandidate(lastError string, retainCurrentPath bool) ChainFailureOutcome {
	if retainCurrentPath {
		return ChainFailureOutcome{
			State:                        "degraded",
			LastError:                    lastError,
			SwitchReason:                 "current_path_reused_after_failure",
			IncrementCurrentPathCounters: true,
		}
	}
	return ChainFailureOutcome{
		State:            "no_candidate",
		LastError:        lastError,
		SwitchReason:     "no_candidate",
		ClearCurrentPath: true,
	}
}
