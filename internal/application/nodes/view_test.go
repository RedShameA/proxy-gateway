package nodes

import "testing"

func TestBuildListItemDerivesStateAndObservationFields(t *testing.T) {
	item := BuildListItem(
		Record{ID: "node-1", Name: "node", Type: "http", Server: "127.0.0.1", ServerPort: 18080, Enabled: true},
		[]SourceRecord{{SourceID: "sub-1", SourceName: "Sub", SourceType: "subscription", DisplayName: "node"}},
		ObservationSnapshot{
			Found:         true,
			Usable:        true,
			EgressIP:      "203.0.113.1",
			EgressCountry: "JP",
			LatencyMS:     42,
			LastSuccessAt: 1000,
			LastFailureAt: 900,
		},
	)

	if item["state"] != "usable" || item["egress_ip"] != "203.0.113.1" || item["observation_latency_ms"] != int64(42) || item["last_observed_at"] != int64(1000) {
		t.Fatalf("item = %#v", item)
	}
	sources, ok := item["sources"].([]map[string]any)
	if !ok || len(sources) != 1 || sources[0]["source_id"] != "sub-1" {
		t.Fatalf("sources = %#v", item["sources"])
	}
}

func TestBuildListItemHidesLatencyForUnusableStaleObservation(t *testing.T) {
	item := BuildListItem(
		Record{ID: "node-1", Name: "node", Type: "http", Server: "127.0.0.1", ServerPort: 18080, Enabled: true},
		nil,
		ObservationSnapshot{
			Found:         true,
			Usable:        false,
			EgressCountry: "US",
			LatencyMS:     2162,
			LastError:     "dial failed",
			LastSuccessAt: 1000,
			LastFailureAt: 2000,
		},
	)

	if item["state"] != "unusable" || item["observation_latency_ms"] != nil || item["last_observed_at"] != int64(2000) {
		t.Fatalf("item = %#v, want unusable without list latency and latest observed time", item)
	}
	observation, ok := item["observation"].(map[string]any)
	if !ok {
		t.Fatalf("observation = %#v, want map", item["observation"])
	}
	if observation["latency_ms"] != int64(2162) || observation["stale"] != true {
		t.Fatalf("observation = %#v, want stale detail with historical latency", observation)
	}
}

func TestNodeState(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		observation ObservationSnapshot
		want        string
	}{
		{name: "disabled", enabled: false, want: "disabled"},
		{name: "pending", enabled: true, want: "pending_observation"},
		{name: "usable", enabled: true, observation: ObservationSnapshot{Found: true, Usable: true}, want: "usable"},
		{name: "unusable", enabled: true, observation: ObservationSnapshot{Found: true}, want: "unusable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NodeState(tt.enabled, tt.observation); got != tt.want {
				t.Fatalf("NodeState = %q, want %q", got, tt.want)
			}
		})
	}
}
