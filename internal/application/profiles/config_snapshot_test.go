package profiles

import (
	"testing"

	domainprofile "proxygateway/internal/domain/profile"
)

func TestConfigRecordDomainSnapshot(t *testing.T) {
	record := ConfigRecord{
		Type:                         domainprofile.TypeChain,
		FixedNodeID:                  "exit_1",
		ExitNodeIDs:                  []string{"exit_1"},
		ChainEvaluationMode:          domainprofile.ChainEvaluationModeChainLink,
		TestURL:                      "https://example.com",
		EgressCountry:                "JP",
		EgressCountryMode:            domainprofile.EgressCountryModeInclude,
		EgressCountries:              []string{"JP"},
		NodeSourceMode:               domainprofile.NodeSourceModeSubscriptions,
		SourceIDs:                    []string{"sub_1"},
		Protocols:                    []string{"vmess"},
		NameIncludeRegex:             "tokyo",
		NameExcludeRegex:             "slow",
		ManualOnly:                   false,
		MinEvaluationIntervalSeconds: 30,
		CandidateLimit:               10,
		RelativeImprovementThreshold: 0.2,
		AbsoluteLatencyImprovementMS: 100,
		CurrentNodeID:                "front_1",
		CurrentExitNodeID:            "exit_1",
		State:                        domainprofile.StateReady,
		CurrentPathLatencyMS:         123,
		SwitchReason:                 domainprofile.SwitchReasonCurrentPathStillBest,
		LastEvaluationDetailsJSON:    "{}",
		AutoEvaluationEnabled:        true,
		AutoEvaluationInterval:       300,
		NodeStickyEnabled:            true,
		ConfigVersion:                7,
	}

	snapshot := record.DomainSnapshot()

	if snapshot.Type != domainprofile.TypeChain || snapshot.FixedNodeID != "exit_1" || snapshot.CurrentNodeID != "front_1" {
		t.Fatalf("snapshot identity/path = %#v", snapshot)
	}
	if snapshot.MinEvaluationIntervalSeconds != 30 || snapshot.AutoEvaluationInterval != 300 || snapshot.ConfigVersion != 7 {
		t.Fatalf("snapshot timing/version = %#v", snapshot)
	}
	if len(snapshot.EgressCountries) != 1 || snapshot.EgressCountries[0] != "JP" {
		t.Fatalf("snapshot countries = %#v", snapshot.EgressCountries)
	}
}

func TestApplyDomainSnapshotUpdatesRuntimeFields(t *testing.T) {
	record := ConfigRecord{
		Type:        domainprofile.TypeFastest,
		FixedNodeID: "node_keep",
		State:       domainprofile.StateRunning,
	}
	snapshot := record.DomainSnapshot()
	snapshot.CurrentNodeID = "node_1"
	snapshot.CurrentExitNodeID = "exit_1"
	snapshot.CurrentPathLatencyMS = 88
	snapshot.SwitchReason = domainprofile.SwitchReasonCandidateClearlyBetter
	snapshot.LastEvaluationDetailsJSON = `{"ok":true}`
	snapshot.State = domainprofile.StateReady
	snapshot.ConfigVersion = 3

	ApplyDomainSnapshot(&record, snapshot)

	if record.Type != domainprofile.TypeFastest || record.FixedNodeID != "node_keep" {
		t.Fatalf("stable config fields changed = %#v", record)
	}
	if record.CurrentNodeID != "node_1" || record.CurrentExitNodeID != "exit_1" || record.State != domainprofile.StateReady {
		t.Fatalf("runtime fields = %#v", record)
	}
	if record.CurrentPathLatencyMS != 88 || record.SwitchReason != domainprofile.SwitchReasonCandidateClearlyBetter || record.ConfigVersion != 3 {
		t.Fatalf("runtime detail fields = %#v", record)
	}
}
