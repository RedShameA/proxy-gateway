package profiles

import (
	"strings"

	appdictionaries "proxygateway/internal/application/dictionaries"
	appnodes "proxygateway/internal/application/nodes"
	domainprofile "proxygateway/internal/domain/profile"
)

type NodePathSummary struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Protocol             string `json:"protocol"`
	Server               string `json:"server"`
	ServerPort           int    `json:"server_port"`
	EgressIP             any    `json:"egress_ip"`
	EgressCountry        any    `json:"egress_country"`
	ObservationLatencyMS any    `json:"observation_latency_ms"`
	LastObservedAt       any    `json:"last_observed_at"`
}

type SinglePathSummary struct {
	PathType    string          `json:"path_type"`
	Node        NodePathSummary `json:"node"`
	LatencyMS   any             `json:"latency_ms"`
	LatencyKind string          `json:"latency_kind"`
	EvaluatedAt any             `json:"evaluated_at"`
}

type ChainPathSummary struct {
	PathType            string          `json:"path_type"`
	FrontNode           NodePathSummary `json:"front_node"`
	ExitNode            NodePathSummary `json:"exit_node"`
	FinalEgressCountry  any             `json:"final_egress_country"`
	ChainEvaluationMode string          `json:"chain_evaluation_mode"`
	LatencyMS           any             `json:"latency_ms"`
	LatencyKind         string          `json:"latency_kind"`
	EvaluatedAt         any             `json:"evaluated_at"`
}

type CurrentPathDeps struct {
	ChainPathMatchesProfile           func(ConfigRecord, string, string) bool
	ProfileNodeMatchesCandidateFilter func(profileID, nodeID string, filter domainprofile.CandidateFilter) bool
	NodePathSummary                   func(nodeID string) (NodePathSummary, bool)
}

func BuildNodePathSummary(node appnodes.Record, observation appnodes.ObservationSnapshot) NodePathSummary {
	var egressIP any
	if observation.EgressIP != "" {
		egressIP = observation.EgressIP
	}
	var latency any
	if observation.LatencyMS > 0 {
		latency = observation.LatencyMS
	}
	var observedAt any
	if observation.LastSuccessAt > 0 {
		observedAt = observation.LastSuccessAt
	}
	return NodePathSummary{
		ID:                   node.ID,
		Name:                 node.Name,
		Protocol:             node.Type,
		Server:               node.Server,
		ServerPort:           node.ServerPort,
		EgressIP:             egressIP,
		EgressCountry:        egressCountryDisplay(observation.EgressCountry),
		ObservationLatencyMS: latency,
		LastObservedAt:       observedAt,
	}
}

func BuildCurrentPath(record ConfigRecord, deps CurrentPathDeps) any {
	if record.Type == domainprofile.TypeChain {
		if record.CurrentNodeID == "" || record.CurrentExitNodeID == "" {
			return nil
		}
		if deps.ChainPathMatchesProfile != nil && !deps.ChainPathMatchesProfile(record, record.CurrentNodeID, record.CurrentExitNodeID) {
			return nil
		}
		frontNode, frontOK := nodePathSummary(record.CurrentNodeID, deps.NodePathSummary)
		exitNode, exitOK := nodePathSummary(record.CurrentExitNodeID, deps.NodePathSummary)
		if !frontOK || !exitOK {
			return nil
		}
		return BuildChainPathSummary(frontNode, exitNode, record.ChainEvaluationMode, record.CurrentPathLatencyMS, record.LastEvaluatedAt)
	}
	if record.CurrentNodeID == "" {
		return nil
	}
	if record.Type == domainprofile.TypeFastest && deps.ProfileNodeMatchesCandidateFilter != nil && !deps.ProfileNodeMatchesCandidateFilter(record.ID, record.CurrentNodeID, record.CandidateFilter()) {
		return nil
	}
	node, ok := nodePathSummary(record.CurrentNodeID, deps.NodePathSummary)
	if !ok {
		return nil
	}
	return BuildSinglePathSummary(node, record.CurrentPathLatencyMS, record.LastEvaluatedAt)
}

func BuildSinglePathSummary(node NodePathSummary, latencyMS int64, evaluatedAt int64) SinglePathSummary {
	return SinglePathSummary{
		PathType:    "single",
		Node:        node,
		LatencyMS:   nullablePositiveInt64(latencyMS),
		LatencyKind: domainprofile.ChainEvaluationModeEndToEnd,
		EvaluatedAt: nullablePositiveInt64(evaluatedAt),
	}
}

func BuildChainPathSummary(frontNode, exitNode NodePathSummary, chainEvaluationMode string, latencyMS int64, evaluatedAt int64) ChainPathSummary {
	mode := domainprofile.NormalizeChainEvaluationMode(chainEvaluationMode)
	return ChainPathSummary{
		PathType:            domainprofile.TypeChain,
		FrontNode:           frontNode,
		ExitNode:            exitNode,
		FinalEgressCountry:  exitNode.EgressCountry,
		ChainEvaluationMode: mode,
		LatencyMS:           nullablePositiveInt64(latencyMS),
		LatencyKind:         mode,
		EvaluatedAt:         nullablePositiveInt64(evaluatedAt),
	}
}

func nodePathSummary(nodeID string, load func(string) (NodePathSummary, bool)) (NodePathSummary, bool) {
	if load == nil {
		return NodePathSummary{}, false
	}
	return load(nodeID)
}

func nullablePositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func egressCountryDisplay(country string) map[string]any {
	country = normalizeEgressCountryValue(country)
	if country == "" || country == "__unknown__" {
		return map[string]any{"value": "__unknown__", "iso_code": nil, "name_zh": "未知", "is_unknown": true}
	}
	return map[string]any{"value": country, "iso_code": country, "name_zh": appdictionaries.CountryNameZH(country), "is_unknown": false}
}

func normalizeEgressCountryValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "__unknown__") {
		return "__unknown__"
	}
	return strings.ToUpper(value)
}
