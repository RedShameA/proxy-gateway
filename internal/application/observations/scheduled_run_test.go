package observations

import "testing"

func TestPlanScheduledAggregateRunSkipsWhenAggregateRunAlreadyUnfinished(t *testing.T) {
	plan := PlanScheduledAggregateRun([]NodeTarget{
		{ID: "node-1", Name: "Node 1"},
	}, "https://probe.example", true)

	if !plan.CreateRun || !plan.FinishImmediately {
		t.Fatalf("plan = %#v, want created immediate-finish run", plan)
	}
	if plan.Scope != "all_nodes" || plan.TriggerSource != "scheduled" {
		t.Fatalf("plan identity = %#v", plan)
	}
	if plan.ProbeURL != "https://probe.example" || len(plan.Targets) != 1 {
		t.Fatalf("plan targets = %#v", plan)
	}
	if plan.Result != "skipped" || plan.ReasonCode != "previous_run_still_running" || plan.NotifyRunner {
		t.Fatalf("plan outcome = %#v", plan)
	}
}

func TestPlanScheduledAggregateRunQueuesWhenTargetsExistAndNoConflict(t *testing.T) {
	plan := PlanScheduledAggregateRun([]NodeTarget{
		{ID: "node-1", Name: "Node 1"},
		{ID: "node-2", Name: "Node 2"},
	}, "https://probe.example", false)

	if !plan.CreateRun || plan.FinishImmediately {
		t.Fatalf("plan = %#v, want queued run", plan)
	}
	if plan.Result != "" || plan.ReasonCode != "" || !plan.NotifyRunner {
		t.Fatalf("plan outcome = %#v", plan)
	}
	if len(plan.Targets) != 2 || plan.Scope != "all_nodes" || plan.TriggerSource != "scheduled" {
		t.Fatalf("plan identity = %#v", plan)
	}
}

func TestPlanScheduledAggregateRunSkipsCreationWhenNoTargetsRemain(t *testing.T) {
	plan := PlanScheduledAggregateRun(nil, "https://probe.example", false)
	if plan.CreateRun || plan.NotifyRunner || plan.FinishImmediately {
		t.Fatalf("plan = %#v, want zero plan", plan)
	}
}
