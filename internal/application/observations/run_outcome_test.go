package observations

import (
	"testing"

	appmaintenance "proxygateway/internal/application/maintenance"
)

func TestBuildNoTargetOutcome(t *testing.T) {
	outcome := BuildNoTargetOutcome()
	if outcome.Result != appmaintenance.ResultSkipped || outcome.ReasonCode != appmaintenance.ReasonNoTargets || outcome.FinishedCount != 0 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if outcome.SuccessCount != 0 || outcome.FailureCount != 0 {
		t.Fatalf("counts = %#v", outcome)
	}
	if len(outcome.SampleFailures) != 0 || len(outcome.FailureReasons) != 0 {
		t.Fatalf("failure details = %#v", outcome)
	}
}

func TestBuildCompletedOutcomeReportsSuccess(t *testing.T) {
	outcome := BuildCompletedOutcome(appmaintenance.TriggerManual, []RunResult{
		{NodeID: "node-1", Name: "Node 1", OK: true},
		{NodeID: "node-2", Name: "Node 2", OK: true},
	})
	if outcome.Result != appmaintenance.ResultSuccess || outcome.ReasonCode != appmaintenance.ReasonCompleted || outcome.FinishedCount != 2 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if outcome.SuccessCount != 2 || outcome.FailureCount != 0 || outcome.EnqueueWaitingProfiles {
		t.Fatalf("counts = %#v", outcome)
	}
}

func TestBuildCompletedOutcomeReportsPartialFailure(t *testing.T) {
	outcome := BuildCompletedOutcome(appmaintenance.TriggerManual, []RunResult{
		{NodeID: "node-1", Name: "Node 1", OK: true},
		{NodeID: "node-2", Name: "Node 2", Error: "dial failed"},
	})
	if outcome.Result != appmaintenance.ResultSuccess || outcome.ReasonCode != appmaintenance.ReasonPartialFailure || outcome.FinishedCount != 2 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if outcome.SuccessCount != 1 || outcome.FailureCount != 1 || outcome.LastError != "dial failed" {
		t.Fatalf("counts = %#v", outcome)
	}
	if outcome.FailureReasons["dial failed"] != 1 {
		t.Fatalf("failure reasons = %#v", outcome.FailureReasons)
	}
	if len(outcome.SampleFailures) != 1 || outcome.SampleFailures[0]["node_id"] != "node-2" {
		t.Fatalf("sample failures = %#v", outcome.SampleFailures)
	}
}

func TestBuildCompletedOutcomeReportsAllFailedAndRefreshFollowUp(t *testing.T) {
	outcome := BuildCompletedOutcome(appmaintenance.TriggerSubscriptionRefresh, []RunResult{
		{NodeID: "node-1", Name: "Node 1", Error: "timeout"},
		{NodeID: "node-2", Name: "Node 2", Error: ""},
	})
	if outcome.Result != appmaintenance.ResultFailure || outcome.ReasonCode != appmaintenance.ReasonAllFailed || outcome.FinishedCount != 2 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if outcome.SuccessCount != 0 || outcome.FailureCount != 2 || !outcome.EnqueueWaitingProfiles {
		t.Fatalf("counts = %#v", outcome)
	}
	if outcome.FailureReasons["timeout"] != 1 || outcome.FailureReasons["observation_failed"] != 1 {
		t.Fatalf("failure reasons = %#v", outcome.FailureReasons)
	}
}
