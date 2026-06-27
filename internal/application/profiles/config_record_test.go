package profiles

import "testing"

func TestConfigRecordAppliesDefaultsAndDerivedValues(t *testing.T) {
	record := ConfigRecord{
		ID:                    "profile_1",
		Type:                  "fixed_node",
		AutoEvaluationEnabled: false,
		CurrentNodeID:         "node_1",
		NodeStickyEnabled:     true,
	}

	record.ApplyDefaults()

	if record.EgressCountryMode != "include" {
		t.Fatalf("EgressCountryMode = %q, want include", record.EgressCountryMode)
	}
	if record.EffectiveProfileIdentifier() != "profile_1" {
		t.Fatalf("EffectiveProfileIdentifier = %q", record.EffectiveProfileIdentifier())
	}
	if record.NodeStickyEnabledForType() {
		t.Fatal("fixed_node should not expose sticky node behavior")
	}
	if record.DynamicStateAfterUpdate() != "ready" {
		t.Fatalf("DynamicStateAfterUpdate = %q, want ready", record.DynamicStateAfterUpdate())
	}
}

func TestConfigRecordCandidateFilterUsesProfileFields(t *testing.T) {
	record := ConfigRecord{
		EgressCountry:     "JP",
		EgressCountryMode: "include",
		EgressCountries:   []string{"JP", "US"},
		NodeSourceMode:    "specific_subscriptions",
		SourceIDs:         []string{"sub_1"},
		Protocols:         []string{"vmess"},
		NameIncludeRegex:  "tokyo",
		NameExcludeRegex:  "slow",
		ManualOnly:        true,
	}

	filter := record.CandidateFilter()

	if filter.EgressCountry != "JP" || filter.EgressCountryMode != "include" {
		t.Fatalf("country filter = %#v", filter)
	}
	if len(filter.SourceIDs) != 1 || filter.SourceIDs[0] != "sub_1" || !filter.ManualOnly {
		t.Fatalf("source filter = %#v", filter)
	}
	if filter.NameIncludeRegex != "tokyo" || filter.NameExcludeRegex != "slow" {
		t.Fatalf("name filter = %#v", filter)
	}
}

func TestStateHasReusablePath(t *testing.T) {
	for _, state := range []string{"ready", "degraded", "running"} {
		if !StateHasReusablePath(state) {
			t.Fatalf("StateHasReusablePath(%q) = false, want true", state)
		}
	}
	for _, state := range []string{"pending", "failed", "waiting_observation", ""} {
		if StateHasReusablePath(state) {
			t.Fatalf("StateHasReusablePath(%q) = true, want false", state)
		}
	}
}
