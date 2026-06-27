package maintenance

import "testing"

func TestBuildProfileEvaluationFinishTreatsSuccessfulSwitchAsSuccess(t *testing.T) {
	finish := BuildProfileEvaluationFinish(ProfileEvaluationFinishInput{
		Detail: map[string]any{
			"config_version": float64(2),
		},
		EvaluationDetail: map[string]any{
			"candidate_count": float64(47),
			"failure_count":   float64(2),
		},
		ProfileID:      "profile_1",
		ProfileState:   "ready",
		CandidateCount: 47,
		OK:             true,
		SwitchReason:   "candidate_clearly_better",
	})

	if finish.Result != ResultSuccess || finish.ReasonCode != "candidate_clearly_better" || finish.LastError != "" {
		t.Fatalf("finish result = %#v", finish)
	}
	if finish.FinishedCount != 47 {
		t.Fatalf("FinishedCount = %d, want 47", finish.FinishedCount)
	}
	if finish.Detail["success_count"] != 45 || finish.Detail["failure_count"] != 2 {
		t.Fatalf("detail counts = %#v", finish.Detail)
	}
	if finish.Detail["switch_decision"] != "candidate_clearly_better" || finish.Detail["current_path_result"] != "ready" {
		t.Fatalf("detail result = %#v", finish.Detail)
	}
}

func TestBuildProfileEvaluationFinishTreatsDegradedFailureAsWarning(t *testing.T) {
	finish := BuildProfileEvaluationFinish(ProfileEvaluationFinishInput{
		ProfileID:      "profile_1",
		ProfileState:   "degraded",
		CandidateCount: 3,
		OK:             false,
		LastError:      "dial failed",
		SwitchReason:   "current_path_reused_after_failure",
	})

	if finish.Result != ResultWarning || finish.ReasonCode != "current_path_reused_after_failure" {
		t.Fatalf("finish result = %#v", finish)
	}
	if finish.LastError != "dial failed" {
		t.Fatalf("LastError = %q", finish.LastError)
	}
	if finish.Detail["success_count"] != 3 || finish.Detail["failure_count"] != 0 {
		t.Fatalf("detail counts = %#v", finish.Detail)
	}
}
