package evaluations

type FastestFailureOutcome struct {
	State                        string
	LastError                    string
	SwitchReason                 string
	ClearCurrentNode             bool
	IncrementCurrentPathCounters bool
}

func PlanFastestNoCandidate(lastError string, retainCurrentPath bool) FastestFailureOutcome {
	if lastError == "" {
		lastError = "no candidate nodes"
	}
	if retainCurrentPath {
		return FastestFailureOutcome{
			State:                        "degraded",
			LastError:                    lastError,
			SwitchReason:                 "current_path_reused_after_failure",
			IncrementCurrentPathCounters: true,
		}
	}
	return FastestFailureOutcome{
		State:            "no_candidate",
		LastError:        lastError,
		SwitchReason:     "no_candidate",
		ClearCurrentNode: true,
	}
}

func PlanFastestAllCandidatesFailed(currentNodeID, lastError string) FastestFailureOutcome {
	if lastError == "" {
		lastError = "all candidates failed"
	}
	if currentNodeID != "" {
		return FastestFailureOutcome{
			State:                        "degraded",
			LastError:                    lastError,
			SwitchReason:                 "current_path_reused_after_failure",
			IncrementCurrentPathCounters: true,
		}
	}
	return FastestFailureOutcome{
		State:            "failed",
		LastError:        lastError,
		SwitchReason:     "all_candidates_failed",
		ClearCurrentNode: true,
	}
}
