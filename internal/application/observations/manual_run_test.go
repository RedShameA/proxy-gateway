package observations

import (
	"errors"
	"testing"
)

func TestPlanManualRunSelectsSingleTargetAndUsesLegacyProbeFallback(t *testing.T) {
	repo := fakeManualRunRepository{
		nodesByID: map[string]NodeTarget{
			"node-1": {ID: "node-1", Name: "Node 1"},
		},
	}

	plan, err := PlanManualRun(repo, ManualRunCommand{
		NodeID:          " node-1 ",
		LegacyTestURL:   "https://legacy.example/probe",
		DefaultProbeURL: "https://default.example/probe",
	})
	if err != nil {
		t.Fatalf("PlanManualRun error = %v", err)
	}
	if plan.Scope != "single_node" || plan.CancelUnfinishedAggregateRuns {
		t.Fatalf("plan scope = %#v", plan)
	}
	if plan.ProbeURL != "https://legacy.example/probe" {
		t.Fatalf("ProbeURL = %q, want legacy fallback", plan.ProbeURL)
	}
	if len(plan.Targets) != 1 || plan.Targets[0].ID != "node-1" {
		t.Fatalf("Targets = %#v", plan.Targets)
	}
}

func TestPlanManualRunTreatsMultipleTargetsAsAggregateAndKeepsOrder(t *testing.T) {
	repo := fakeManualRunRepository{
		nodesByID: map[string]NodeTarget{
			"node-2": {ID: "node-2", Name: "Node 2"},
			"node-1": {ID: "node-1", Name: "Node 1"},
		},
	}

	plan, err := PlanManualRun(repo, ManualRunCommand{
		NodeIDs:         []string{" node-2 ", "", "node-1", "node-2", "missing"},
		ProbeURL:        "https://explicit.example/probe",
		DefaultProbeURL: "https://default.example/probe",
	})
	if err != nil {
		t.Fatalf("PlanManualRun error = %v", err)
	}
	if plan.Scope != "all_nodes" || !plan.CancelUnfinishedAggregateRuns {
		t.Fatalf("plan scope = %#v", plan)
	}
	if plan.ProbeURL != "https://explicit.example/probe" {
		t.Fatalf("ProbeURL = %q, want explicit", plan.ProbeURL)
	}
	if len(plan.Targets) != 2 || plan.Targets[0].ID != "node-2" || plan.Targets[1].ID != "node-1" {
		t.Fatalf("Targets = %#v", plan.Targets)
	}
}

func TestPlanManualRunFallsBackToAllEnabledTargetsAndDefaultProbeURL(t *testing.T) {
	repo := fakeManualRunRepository{
		allNodes: []NodeTarget{
			{ID: "node-1", Name: "Node 1"},
			{ID: "node-2", Name: "Node 2"},
		},
	}

	plan, err := PlanManualRun(repo, ManualRunCommand{
		DefaultProbeURL: "https://default.example/probe",
	})
	if err != nil {
		t.Fatalf("PlanManualRun error = %v", err)
	}
	if plan.Scope != "all_nodes" || !plan.CancelUnfinishedAggregateRuns {
		t.Fatalf("plan scope = %#v", plan)
	}
	if plan.ProbeURL != "https://default.example/probe" {
		t.Fatalf("ProbeURL = %q, want default", plan.ProbeURL)
	}
	if len(plan.Targets) != 2 {
		t.Fatalf("Targets = %#v, want all enabled targets", plan.Targets)
	}
}

func TestPlanManualRunReturnsNotFoundForMissingSingleTarget(t *testing.T) {
	_, err := PlanManualRun(fakeManualRunRepository{}, ManualRunCommand{
		NodeIDs:         []string{"missing"},
		DefaultProbeURL: "https://default.example/probe",
	})
	if !errors.Is(err, ErrObservationTargetNotFound) {
		t.Fatalf("error = %v, want ErrObservationTargetNotFound", err)
	}
}

type fakeManualRunRepository struct {
	nodesByID map[string]NodeTarget
	allNodes  []NodeTarget
}

func (f fakeManualRunRepository) EnabledNodeByID(nodeID string) (NodeTarget, bool, error) {
	node, ok := f.nodesByID[nodeID]
	return node, ok, nil
}

func (f fakeManualRunRepository) AllEnabledNodes() ([]NodeTarget, error) {
	return append([]NodeTarget{}, f.allNodes...), nil
}
