package profiles

import "testing"

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
