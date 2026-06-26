package profiles

import domainprofile "proxygateway/internal/domain/profile"

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

func BuildSinglePathSummary(node NodePathSummary, latencyMS int64, evaluatedAt int64) SinglePathSummary {
	return SinglePathSummary{
		PathType:    "single",
		Node:        node,
		LatencyMS:   nullablePositiveInt64(latencyMS),
		LatencyKind: "end_to_end",
		EvaluatedAt: nullablePositiveInt64(evaluatedAt),
	}
}

func BuildChainPathSummary(frontNode, exitNode NodePathSummary, chainEvaluationMode string, latencyMS int64, evaluatedAt int64) ChainPathSummary {
	mode := domainprofile.NormalizeChainEvaluationMode(chainEvaluationMode)
	return ChainPathSummary{
		PathType:            "chain",
		FrontNode:           frontNode,
		ExitNode:            exitNode,
		FinalEgressCountry:  exitNode.EgressCountry,
		ChainEvaluationMode: mode,
		LatencyMS:           nullablePositiveInt64(latencyMS),
		LatencyKind:         mode,
		EvaluatedAt:         nullablePositiveInt64(evaluatedAt),
	}
}

func nullablePositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
