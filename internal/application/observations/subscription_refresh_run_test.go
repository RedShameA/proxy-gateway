package observations

import "testing"

func TestPlanSubscriptionRefreshAggregateRunQueuesAllNodesAndNotifiesRunner(t *testing.T) {
	plan := PlanSubscriptionRefreshAggregateRun([]NodeTarget{
		{ID: "node-1", Name: "Node 1"},
		{ID: "node-2", Name: "Node 2"},
	}, "https://probe.example")

	if !plan.CreateRun || plan.FinishImmediately {
		t.Fatalf("plan = %#v, want queued run", plan)
	}
	if plan.TriggerSource != "subscription_refresh" || plan.Scope != "all_nodes" {
		t.Fatalf("plan identity = %#v", plan)
	}
	if plan.ProbeURL != "https://probe.example" || !plan.NotifyRunner || len(plan.Targets) != 2 {
		t.Fatalf("plan targets = %#v", plan)
	}
}

func TestPlanSubscriptionRefreshAggregateRunSkipsCreationWithoutTargets(t *testing.T) {
	plan := PlanSubscriptionRefreshAggregateRun(nil, "https://probe.example")
	if plan.CreateRun || plan.NotifyRunner || plan.FinishImmediately {
		t.Fatalf("plan = %#v, want zero plan", plan)
	}
}
