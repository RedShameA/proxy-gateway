package profiles

import (
	"testing"

	appnodes "proxygateway/internal/application/nodes"
	domainprofile "proxygateway/internal/domain/profile"
)

func TestBuildNodePathSummary(t *testing.T) {
	node := appnodes.Record{
		ID:         "node_1",
		Name:       "tokyo",
		Type:       "socks5",
		Server:     "127.0.0.1",
		ServerPort: 1080,
	}
	observation := appnodes.ObservationSnapshot{
		Found:         true,
		Usable:        true,
		EgressIP:      "203.0.113.10",
		EgressCountry: "jp",
		LatencyMS:     42,
		LastSuccessAt: 1234,
	}

	summary := BuildNodePathSummary(node, observation)
	if summary.ID != "node_1" || summary.Name != "tokyo" || summary.Protocol != "socks5" {
		t.Fatalf("summary node fields = %#v", summary)
	}
	if summary.Server != "127.0.0.1" || summary.ServerPort != 1080 {
		t.Fatalf("summary server fields = %#v", summary)
	}
	if summary.EgressIP != "203.0.113.10" || summary.ObservationLatencyMS != int64(42) || summary.LastObservedAt != int64(1234) {
		t.Fatalf("summary observation fields = %#v", summary)
	}
	country := countryDisplayMap(t, summary.EgressCountry)
	if country["value"] != "JP" || country["iso_code"] != "JP" || country["name_zh"] != "日本" || country["is_unknown"] != false {
		t.Fatalf("country display = %#v", country)
	}
}

func TestBuildNodePathSummaryUnknownCountry(t *testing.T) {
	summary := BuildNodePathSummary(appnodes.Record{ID: "node_1"}, appnodes.ObservationSnapshot{Found: true, EgressCountry: " __unknown__ "})

	country := countryDisplayMap(t, summary.EgressCountry)
	if country["value"] != "__unknown__" || country["iso_code"] != nil || country["name_zh"] != "未知" || country["is_unknown"] != true {
		t.Fatalf("country display = %#v", country)
	}
}

func TestBuildNodePathSummaryWithoutObservationValues(t *testing.T) {
	summary := BuildNodePathSummary(appnodes.Record{ID: "node_1"}, appnodes.ObservationSnapshot{})

	if summary.EgressIP != nil || summary.ObservationLatencyMS != nil || summary.LastObservedAt != nil {
		t.Fatalf("empty observation fields should be nil: %#v", summary)
	}
	country := countryDisplayMap(t, summary.EgressCountry)
	if country["value"] != "__unknown__" || country["is_unknown"] != true {
		t.Fatalf("country display = %#v", country)
	}
}

func countryDisplayMap(t *testing.T, value any) map[string]any {
	t.Helper()
	country, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("country display type = %T", value)
	}
	return country
}

func TestBuildPathSummaries(t *testing.T) {
	node := NodePathSummary{
		ID:       "node_1",
		Name:     "tokyo",
		Protocol: "socks5",
		Server:   "127.0.0.1",
	}

	single := BuildSinglePathSummary(node, 123, 456)
	if single.PathType != "single" || single.Node.ID != "node_1" {
		t.Fatalf("single path = %#v", single)
	}
	if single.LatencyMS != int64(123) || single.LatencyKind != "end_to_end" || single.EvaluatedAt != int64(456) {
		t.Fatalf("single latency/evaluated = %#v", single)
	}

	chain := BuildChainPathSummary(node, node, "chain_link", 0, 0)
	if chain.PathType != "chain" || chain.LatencyMS != nil || chain.EvaluatedAt != nil {
		t.Fatalf("chain path = %#v", chain)
	}
	if chain.ChainEvaluationMode != "chain_link" || chain.LatencyKind != "chain_link" {
		t.Fatalf("chain mode/kind = %#v", chain)
	}
}

func TestBuildCurrentPathRejectsFastestPathOutsideCandidateFilter(t *testing.T) {
	cfg := ConfigRecord{
		ID:            "profile_1",
		Type:          "fastest",
		State:         "ready",
		CurrentNodeID: "node_current",
	}

	path := BuildCurrentPath(cfg, CurrentPathDeps{
		ProfileNodeMatchesCandidateFilter: func(profileID, nodeID string, filter domainprofile.CandidateFilter) bool {
			if profileID != "profile_1" || nodeID != "node_current" {
				t.Fatalf("filter check = %s / %s", profileID, nodeID)
			}
			return false
		},
		NodePathSummary: func(nodeID string) (NodePathSummary, bool) {
			t.Fatal("NodePathSummary should not be called when current node fails candidate filter")
			return NodePathSummary{}, false
		},
	})
	if path != nil {
		t.Fatalf("BuildCurrentPath = %#v, want nil", path)
	}
}

func TestBuildCurrentPathBuildsChainPath(t *testing.T) {
	cfg := ConfigRecord{
		ID:                           "profile_1",
		Type:                         "chain",
		State:                        "ready",
		CurrentNodeID:                "front_1",
		CurrentExitNodeID:            "exit_1",
		ChainEvaluationMode:          "chain_link",
		CurrentPathLatencyMS:         123,
		LastEvaluatedAt:              456,
		RelativeImprovementThreshold: 0.2,
	}

	path := BuildCurrentPath(cfg, CurrentPathDeps{
		ChainPathMatchesProfile: func(cfg ConfigRecord, frontNodeID, exitNodeID string) bool {
			if frontNodeID != "front_1" || exitNodeID != "exit_1" {
				t.Fatalf("chain path = %s -> %s", frontNodeID, exitNodeID)
			}
			return true
		},
		NodePathSummary: func(nodeID string) (NodePathSummary, bool) {
			return NodePathSummary{ID: nodeID}, true
		},
	})
	chain, ok := path.(ChainPathSummary)
	if !ok {
		t.Fatalf("BuildCurrentPath type = %T", path)
	}
	if chain.FrontNode.ID != "front_1" || chain.ExitNode.ID != "exit_1" {
		t.Fatalf("chain path = %#v", chain)
	}
	if chain.LatencyMS != int64(123) || chain.EvaluatedAt != int64(456) {
		t.Fatalf("chain timing = %#v", chain)
	}
}

func TestBuildCurrentPathRejectsStaleChainPath(t *testing.T) {
	cfg := ConfigRecord{
		Type:              "chain",
		State:             "ready",
		CurrentNodeID:     "front_1",
		CurrentExitNodeID: "exit_1",
	}

	path := BuildCurrentPath(cfg, CurrentPathDeps{
		ChainPathMatchesProfile: func(cfg ConfigRecord, frontNodeID, exitNodeID string) bool {
			return false
		},
		NodePathSummary: func(nodeID string) (NodePathSummary, bool) {
			t.Fatal("NodePathSummary should not be called for stale chain path")
			return NodePathSummary{}, false
		},
	})
	if path != nil {
		t.Fatalf("BuildCurrentPath = %#v, want nil", path)
	}
}
