package observations

import "testing"

func TestBuildRunCreatePlanForSingleNode(t *testing.T) {
	plan := BuildRunCreatePlan(ScopeSingleNode, []NodeTarget{{ID: "node_1", Name: "Node 1"}}, "https://probe.test")

	if plan.TargetID != "node_1" || plan.TargetLabel != "Node 1" || plan.TotalCount != 1 {
		t.Fatalf("plan = %#v", plan)
	}
	if plan.Detail["target_scope"] != ScopeSingleNode || plan.Detail["probe_url"] != "https://probe.test" {
		t.Fatalf("detail = %#v", plan.Detail)
	}
	ids, ok := plan.Detail["node_ids"].([]string)
	if !ok || len(ids) != 1 || ids[0] != "node_1" {
		t.Fatalf("node_ids = %#v", plan.Detail["node_ids"])
	}
}

func TestNodeIDsFromRunDetailFallsBackToTargetID(t *testing.T) {
	ids := NodeIDsFromRunDetail(" node_legacy ", map[string]any{})

	if len(ids) != 1 || ids[0] != "node_legacy" {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestEffectiveProbeURLReturnsFirstNonBlankValue(t *testing.T) {
	if got := EffectiveProbeURL(" ", "https://legacy.test", "https://default.test"); got != "https://legacy.test" {
		t.Fatalf("EffectiveProbeURL = %q", got)
	}
}
