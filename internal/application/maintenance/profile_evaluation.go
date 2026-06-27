package maintenance

type ProfileEvaluationFinishInput struct {
	Detail           map[string]any
	EvaluationDetail map[string]any
	ProfileID        string
	ProfileState     string
	CandidateCount   int
	OK               bool
	LastError        string
	SwitchReason     string
}

type ProfileEvaluationFinish struct {
	Result        string
	ReasonCode    string
	FinishedCount int
	Detail        map[string]any
	LastError     string
}

func BuildProfileEvaluationFinish(input ProfileEvaluationFinishInput) ProfileEvaluationFinish {
	detail := copyDetail(input.Detail)
	for key, value := range input.EvaluationDetail {
		detail[key] = value
	}
	candidateCount := IntFromDetail(detail["candidate_count"], input.CandidateCount)
	failureCount := IntFromDetail(detail["failure_count"], 0)
	if failureCount < 0 {
		failureCount = 0
	}
	successCount := candidateCount - failureCount
	if successCount < 0 {
		successCount = 0
	}
	detail["profile_id"] = input.ProfileID
	detail["profile_state"] = input.ProfileState
	detail["success_count"] = successCount
	detail["failure_count"] = failureCount
	detail["current_path_result"] = input.ProfileState
	if input.SwitchReason != "" {
		detail["switch_decision"] = input.SwitchReason
	}
	result := ResultSuccess
	reasonCode := firstNonEmpty(input.SwitchReason, ReasonCompleted)
	lastError := ""
	if !input.OK {
		lastError = input.LastError
		if input.ProfileState == "degraded" {
			result = ResultWarning
			reasonCode = firstNonEmpty(input.SwitchReason, ReasonCurrentPathDegraded)
		} else {
			result = ResultFailure
			reasonCode = firstNonEmpty(input.SwitchReason, ReasonEvaluationFailed)
		}
	}
	return ProfileEvaluationFinish{
		Result:        result,
		ReasonCode:    reasonCode,
		FinishedCount: candidateCount,
		Detail:        detail,
		LastError:     lastError,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
