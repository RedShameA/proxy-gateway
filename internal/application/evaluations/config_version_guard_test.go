package evaluations

import "testing"

func TestConfigVersionGuardCancelsWhenRequestedVersionIsSuperseded(t *testing.T) {
	guard := CheckConfigVersion(ConfigVersionGuardInput{
		RequestedConfigVersion: 3,
		CurrentConfigVersion:   4,
	})

	if !guard.Superseded {
		t.Fatalf("Superseded = false, want true")
	}
	if guard.ReasonCode != "superseded_by_config_version" {
		t.Fatalf("ReasonCode = %q", guard.ReasonCode)
	}
	if guard.CurrentConfigVersion != 4 {
		t.Fatalf("CurrentConfigVersion = %d", guard.CurrentConfigVersion)
	}
}

func TestConfigVersionGuardAllowsZeroRequestedVersion(t *testing.T) {
	guard := CheckConfigVersion(ConfigVersionGuardInput{
		RequestedConfigVersion: 0,
		CurrentConfigVersion:   4,
	})

	if guard.Superseded || guard.ReasonCode != "" {
		t.Fatalf("guard = %#v, want allowed", guard)
	}
}
