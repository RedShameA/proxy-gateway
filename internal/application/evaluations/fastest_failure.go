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
			State:                        ProfileStateDegraded,
			LastError:                    lastError,
			SwitchReason:                 SwitchReasonCurrentPathReusedAfterFailure,
			IncrementCurrentPathCounters: true,
		}
	}
	return FastestFailureOutcome{
		State:            ProfileStateNoCandidate,
		LastError:        lastError,
		SwitchReason:     SwitchReasonNoCandidate,
		ClearCurrentNode: true,
	}
}

func PlanFastestAllCandidatesFailed(currentNodeID, lastError string) FastestFailureOutcome {
	if lastError == "" {
		lastError = "all candidates failed"
	}
	if currentNodeID != "" {
		return FastestFailureOutcome{
			State:                        ProfileStateDegraded,
			LastError:                    lastError,
			SwitchReason:                 SwitchReasonCurrentPathReusedAfterFailure,
			IncrementCurrentPathCounters: true,
		}
	}
	return FastestFailureOutcome{
		State:            ProfileStateFailed,
		LastError:        lastError,
		SwitchReason:     SwitchReasonAllCandidatesFailed,
		ClearCurrentNode: true,
	}
}
