package profiles

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("profile_1")

	if cfg.ID != "profile_1" || cfg.Type != "fastest" || cfg.State != "pending" {
		t.Fatalf("default identity/state = %#v", cfg)
	}
	if cfg.EgressCountryMode != "include" || cfg.NodeSourceMode != "all" {
		t.Fatalf("default filter modes = %#v", cfg)
	}
	if !cfg.AutoEvaluationEnabled || cfg.ConfigVersion != 1 {
		t.Fatalf("default evaluation fields = %#v", cfg)
	}
	if cfg.RelativeImprovementThreshold != 0.2 || cfg.AbsoluteLatencyImprovementMS != 100 {
		t.Fatalf("default switching tolerance = %#v", cfg)
	}
}

func TestApplyConfigPatchAppliesCandidateFilter(t *testing.T) {
	cfg := DefaultConfig("profile_1")
	sourceMode := "selected_sources"
	req := PatchRequest{
		CandidateFilter: &PatchCandidateFilter{
			SourceMode:        sourceMode,
			SourceIDs:         []string{"sub_1"},
			Protocols:         []string{"vmess"},
			NameInclude:       "jp",
			NameExclude:       "slow",
			EgressCountryMode: "exclude",
			EgressCountries:   []string{"CN"},
		},
	}

	ApplyConfigPatch(&cfg, req)

	if cfg.NodeSourceMode != "specific_subscriptions" {
		t.Fatalf("NodeSourceMode = %q, want specific_subscriptions", cfg.NodeSourceMode)
	}
	if len(cfg.SourceIDs) != 1 || cfg.SourceIDs[0] != "sub_1" {
		t.Fatalf("SourceIDs = %#v", cfg.SourceIDs)
	}
	if len(cfg.Protocols) != 1 || cfg.Protocols[0] != "vmess" {
		t.Fatalf("Protocols = %#v", cfg.Protocols)
	}
	if cfg.NameIncludeRegex != "jp" || cfg.NameExcludeRegex != "slow" || cfg.EgressCountryMode != "exclude" {
		t.Fatalf("filter fields = %#v", cfg)
	}
}

func TestApplyConfigPatchAppliesEvaluationScheduleModes(t *testing.T) {
	cfg := DefaultConfig("profile_1")
	interval := 600
	ApplyConfigPatch(&cfg, PatchRequest{EvaluationSchedule: &PatchEvaluationSchedule{Mode: "custom", IntervalSeconds: &interval}})
	if !cfg.AutoEvaluationEnabled || cfg.AutoEvaluationInterval != 600 {
		t.Fatalf("custom schedule = %#v", cfg)
	}

	ApplyConfigPatch(&cfg, PatchRequest{EvaluationSchedule: &PatchEvaluationSchedule{Mode: "disabled"}})
	if cfg.AutoEvaluationEnabled {
		t.Fatalf("disabled schedule = %#v", cfg)
	}

	ApplyConfigPatch(&cfg, PatchRequest{EvaluationSchedule: &PatchEvaluationSchedule{Mode: "inherit"}})
	if !cfg.AutoEvaluationEnabled || cfg.AutoEvaluationInterval != 0 {
		t.Fatalf("inherit schedule = %#v", cfg)
	}
}
