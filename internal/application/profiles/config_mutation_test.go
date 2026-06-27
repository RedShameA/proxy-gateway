package profiles

import "testing"

func TestBuildCreateConfigPlanReturnsConfigAndSideEffects(t *testing.T) {
	name := "Work"
	countries := []string{" jp ", "__unknown__"}

	plan, err := BuildCreateConfigPlan("profile_1", PatchRequest{
		Name:            &name,
		EgressCountries: &countries,
	}, configMutationTestDeps())
	if err != nil {
		t.Fatalf("BuildCreateConfigPlan = %v", err)
	}
	if plan.Config.Name != "Work" || plan.Config.ProfileIdentifier != "profile_1" {
		t.Fatalf("config identity = %#v", plan.Config)
	}
	if !plan.EnqueueEvaluation {
		t.Fatalf("EnqueueEvaluation = false, want true")
	}
	if !plan.EnqueueUnknownCountryObservation {
		t.Fatalf("EnqueueUnknownCountryObservation = false, want true")
	}
	if got := plan.Config.EgressCountries; len(got) != 2 || got[0] != "JP" || got[1] != "__unknown__" {
		t.Fatalf("EgressCountries = %#v", got)
	}
}

func TestBuildUpdateConfigPlanResetsPathAndReportsSideEffects(t *testing.T) {
	countries := []string{"JP"}
	original := DefaultConfig("profile_1")
	original.Name = "Work"
	original.EgressCountries = []string{"US"}
	original.CurrentNodeID = "node_current"
	original.State = "ready"
	original.CurrentPathLatencyMS = 123
	original.NodeStickyEnabled = true
	original.ConfigVersion = 7

	plan, err := BuildUpdateConfigPlan(original, PatchRequest{EgressCountries: &countries}, configMutationTestDeps())
	if err != nil {
		t.Fatalf("BuildUpdateConfigPlan = %v", err)
	}
	if !plan.EvaluationChanged || !plan.ResetCurrentPath || !plan.ReleaseRetainedNodes || !plan.EnqueueEvaluation || !plan.EnqueueUnknownCountryObservation {
		t.Fatalf("plan flags = %#v", plan)
	}
	if plan.Config.ConfigVersion != 8 {
		t.Fatalf("ConfigVersion = %d, want 8", plan.Config.ConfigVersion)
	}
	if plan.Config.CurrentNodeID != "" || plan.Config.CurrentPathLatencyMS != 0 || plan.Config.State != "running" {
		t.Fatalf("current path was not reset: %#v", plan.Config)
	}
}

func configMutationTestDeps() ConfigValidationDeps {
	return ConfigValidationDeps{
		DefaultTestURL: "https://example.test/generate_204",
		NodeExists: func(string) (bool, error) {
			return true, nil
		},
	}
}
