package profile

import "testing"

func TestPlanConfigUpdateResetsDynamicPathForPathSelectionChange(t *testing.T) {
	original := ConfigSnapshot{
		Type:                      "fastest",
		EgressCountries:           []string{"US"},
		NodeSourceMode:            "all",
		AutoEvaluationEnabled:     true,
		CurrentNodeID:             "node-current",
		CurrentExitNodeID:         "exit-current",
		CurrentPathLatencyMS:      123,
		State:                     "ready",
		SwitchReason:              "fastest",
		LastEvaluationDetailsJSON: `{"selected":"node-current"}`,
		ConfigVersion:             7,
	}
	updated := original
	updated.EgressCountries = []string{"JP"}

	plan := PlanConfigUpdate(original, updated)

	if !plan.EvaluationChanged {
		t.Fatalf("EvaluationChanged = false, want true")
	}
	if !plan.ResetCurrentPath {
		t.Fatalf("ResetCurrentPath = false, want true")
	}
	if !plan.EnqueueEvaluation {
		t.Fatalf("EnqueueEvaluation = false, want true")
	}
	if !plan.EnqueueUnknownCountryObservation {
		t.Fatalf("EnqueueUnknownCountryObservation = false, want true")
	}
	if plan.Config.ConfigVersion != 8 {
		t.Fatalf("ConfigVersion = %d, want 8", plan.Config.ConfigVersion)
	}
	if plan.Config.CurrentNodeID != "" || plan.Config.CurrentExitNodeID != "" || plan.Config.CurrentPathLatencyMS != 0 {
		t.Fatalf("current path not reset: %#v", plan.Config)
	}
	if plan.Config.SwitchReason != "access_profile_change" {
		t.Fatalf("SwitchReason = %q, want access_profile_change", plan.Config.SwitchReason)
	}
	if plan.Config.LastEvaluationDetailsJSON != "{}" {
		t.Fatalf("LastEvaluationDetailsJSON = %q, want {}", plan.Config.LastEvaluationDetailsJSON)
	}
	if plan.Config.State != "running" {
		t.Fatalf("State = %q, want running", plan.Config.State)
	}
}

func TestPlanConfigUpdateKeepsCurrentPathForEvaluationTimingChange(t *testing.T) {
	original := ConfigSnapshot{
		Type:                         "fastest",
		MinEvaluationIntervalSeconds: 60,
		AutoEvaluationEnabled:        true,
		CurrentNodeID:                "node-current",
		State:                        "ready",
		CurrentPathLatencyMS:         123,
		ConfigVersion:                7,
	}
	updated := original
	updated.MinEvaluationIntervalSeconds = 120

	plan := PlanConfigUpdate(original, updated)

	if !plan.EvaluationChanged {
		t.Fatalf("EvaluationChanged = false, want true")
	}
	if plan.ResetCurrentPath {
		t.Fatalf("ResetCurrentPath = true, want false")
	}
	if !plan.EnqueueEvaluation {
		t.Fatalf("EnqueueEvaluation = false, want true")
	}
	if plan.Config.ConfigVersion != 8 {
		t.Fatalf("ConfigVersion = %d, want 8", plan.Config.ConfigVersion)
	}
	if plan.Config.CurrentNodeID != "node-current" || plan.Config.CurrentPathLatencyMS != 123 || plan.Config.State != "ready" {
		t.Fatalf("current path changed unexpectedly: %#v", plan.Config)
	}
}
