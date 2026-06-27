package maintenance

import (
	"testing"

	domainprofile "proxygateway/internal/domain/profile"
)

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
		ProfileState:   domainprofile.StateReady,
		CandidateCount: 47,
		OK:             true,
		SwitchReason:   domainprofile.SwitchReasonCandidateClearlyBetter,
	})

	if finish.Result != ResultSuccess || finish.ReasonCode != domainprofile.SwitchReasonCandidateClearlyBetter || finish.LastError != "" {
		t.Fatalf("finish result = %#v", finish)
	}
	if finish.FinishedCount != 47 {
		t.Fatalf("FinishedCount = %d, want 47", finish.FinishedCount)
	}
	if finish.Detail["success_count"] != 45 || finish.Detail["failure_count"] != 2 {
		t.Fatalf("detail counts = %#v", finish.Detail)
	}
	if finish.Detail["switch_decision"] != domainprofile.SwitchReasonCandidateClearlyBetter || finish.Detail["current_path_result"] != domainprofile.StateReady {
		t.Fatalf("detail result = %#v", finish.Detail)
	}
}

func TestBuildProfileEvaluationFinishTreatsDegradedFailureAsWarning(t *testing.T) {
	finish := BuildProfileEvaluationFinish(ProfileEvaluationFinishInput{
		ProfileID:      "profile_1",
		ProfileState:   domainprofile.StateDegraded,
		CandidateCount: 3,
		OK:             false,
		LastError:      "dial failed",
		SwitchReason:   domainprofile.SwitchReasonCurrentPathReusedAfterFailure,
	})

	if finish.Result != ResultWarning || finish.ReasonCode != domainprofile.SwitchReasonCurrentPathReusedAfterFailure {
		t.Fatalf("finish result = %#v", finish)
	}
	if finish.LastError != "dial failed" {
		t.Fatalf("LastError = %q", finish.LastError)
	}
	if finish.Detail["success_count"] != 3 || finish.Detail["failure_count"] != 0 {
		t.Fatalf("detail counts = %#v", finish.Detail)
	}
}
