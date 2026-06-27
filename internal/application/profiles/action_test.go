package profiles

import (
	"errors"
	"testing"
)

func TestBuildActionPlanQueuesEvaluableProfile(t *testing.T) {
	plan, err := BuildActionPlan(ConfigRecord{Type: "fastest"}, ActionEvaluate)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.EnqueueEvaluation || plan.CreateSwitchRun || plan.ResponseState != "queued" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildActionPlanRejectsFixedProfileEvaluation(t *testing.T) {
	_, err := BuildActionPlan(ConfigRecord{Type: "fixed_node"}, ActionEvaluate)
	if !errors.Is(err, ErrProfileTypeNotEvaluable) {
		t.Fatalf("err = %v, want ErrProfileTypeNotEvaluable", err)
	}
}

func TestBuildActionPlanCreatesManualSwitchPlan(t *testing.T) {
	plan, err := BuildActionPlan(ConfigRecord{
		ID:            "profile_1",
		Type:          "fastest",
		CurrentNodeID: "node_1",
		ConfigVersion: 7,
	}, ActionSwitchToBestObserved)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CreateSwitchRun || !plan.EnqueueEvaluation || plan.ResponseState != "finished" {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.SwitchReason != ManualSwitchReason || plan.SwitchRunDetail["profile_id"] != "profile_1" || plan.SwitchRunDetail["config_version"] != int64(7) {
		t.Fatalf("switch fields = %#v", plan)
	}
}

func TestBuildActionPlanRejectsSwitchWithoutCurrentPath(t *testing.T) {
	_, err := BuildActionPlan(ConfigRecord{Type: "fastest"}, ActionSwitchToBestObserved)
	if !errors.Is(err, ErrNoCurrentPathToSwitch) {
		t.Fatalf("err = %v, want ErrNoCurrentPathToSwitch", err)
	}
}
