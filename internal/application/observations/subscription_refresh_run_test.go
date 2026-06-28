package observations

import (
	"testing"

	appmaintenance "proxygateway/internal/application/maintenance"
)

func TestPlanSubscriptionRefreshAggregateRunQueuesAllNodesAndNotifiesRunner(t *testing.T) {
	plan := PlanSubscriptionRefreshAggregateRun([]NodeTarget{
		{ID: "node-1", Name: "Node 1"},
		{ID: "node-2", Name: "Node 2"},
	}, "https://probe.example")

	if !plan.CreateRun || plan.FinishImmediately {
		t.Fatalf("plan = %#v, want queued run", plan)
	}
	if plan.TriggerSource != appmaintenance.TriggerSubscriptionRefresh || plan.Scope != appmaintenance.NodeObservationScopeAllNodes {
		t.Fatalf("plan identity = %#v", plan)
	}
	if plan.ProbeURL != "https://probe.example" || !plan.NotifyRunner || len(plan.Targets) != 2 {
		t.Fatalf("plan targets = %#v", plan)
	}
}

func TestPlanSubscriptionObservationAggregateRunUsesProvidedTrigger(t *testing.T) {
	plan := PlanSubscriptionObservationAggregateRun([]NodeTarget{
		{ID: "node-1", Name: "Node 1"},
	}, "https://probe.example", appmaintenance.TriggerSubscriptionImport)

	if !plan.CreateRun || !plan.NotifyRunner {
		t.Fatalf("plan = %#v, want queued notifying run", plan)
	}
	if plan.TriggerSource != appmaintenance.TriggerSubscriptionImport || plan.Scope != appmaintenance.NodeObservationScopeAllNodes {
		t.Fatalf("plan identity = %#v", plan)
	}
}

func TestPlanSubscriptionRefreshAggregateRunSkipsCreationWithoutTargets(t *testing.T) {
	plan := PlanSubscriptionRefreshAggregateRun(nil, "https://probe.example")
	if plan.CreateRun || plan.NotifyRunner || plan.FinishImmediately {
		t.Fatalf("plan = %#v, want zero plan", plan)
	}
}
