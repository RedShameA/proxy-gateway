package profiles

import (
	"reflect"
	"testing"
)

func TestSummaryInputFromConfigMapsRecordAndCounts(t *testing.T) {
	currentPath := map[string]any{"path_type": "single"}
	input := SummaryInputFromConfig(ConfigRecord{
		ID:                           "profile_1",
		Name:                         "Work",
		Type:                         "fastest",
		State:                        "ready",
		ProfileIdentifier:            "work",
		CurrentNodeID:                "node_1",
		NodeSourceMode:               "manual",
		EgressCountry:                "JP",
		EgressCountryMode:            "include",
		EgressCountries:              []string{"JP"},
		NameIncludeRegex:             "tokyo",
		CandidateLimit:               5,
		MinEvaluationIntervalSeconds: 60,
		AutoEvaluationEnabled:        true,
		AutoEvaluationInterval:       120,
		NodeStickyEnabled:            true,
		ConfigVersion:                3,
		LastEvaluatedAt:              456,
		LastError:                    "last error",
		SwitchReason:                 "candidate_better",
	}, currentPath, CredentialCounts{Total: 2, Enabled: 1})

	if input.ID != "profile_1" || input.Name != "Work" || !reflect.DeepEqual(input.CurrentPath, currentPath) {
		t.Fatalf("identity/path = %#v", input)
	}
	if input.ProxyCredentialsCount != 2 || input.EnabledCredentialsCount != 1 {
		t.Fatalf("credential counts = %d/%d, want 2/1", input.ProxyCredentialsCount, input.EnabledCredentialsCount)
	}
	if input.EgressCountries[0] != "JP" || input.NameIncludeRegex != "tokyo" {
		t.Fatalf("filter fields = %#v", input)
	}
	if input.LastEvaluatedAt != 456 || input.SwitchReason != "candidate_better" || input.LastError != "last error" {
		t.Fatalf("evaluation fields = %#v", input)
	}
}
