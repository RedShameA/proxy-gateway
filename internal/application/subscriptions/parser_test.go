package subscriptions

import "testing"

func TestParseNodesClashProxyGroupsAreSkipped(t *testing.T) {
	nodes, summary, err := ParseNodes([]byte(`
proxies:
  - name: clash-http-ok
    type: http
    server: 127.0.0.1
    port: 18084
  - name: clash-missing-server
    type: http
    port: 18085
proxy-groups:
  - name: auto
    type: url-test
    proxies:
      - clash-http-ok
`))
	if err != nil {
		t.Fatalf("ParseNodes error = %v", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "clash-http-ok" || nodes[0].Type != "http" {
		t.Fatalf("nodes = %#v, want one imported http node", nodes)
	}
	if summary.Count() != 2 {
		t.Fatalf("summary.Count() = %d, want 2", summary.Count())
	}
	assertSummaryReasonCount(t, summary.Rows(), "missing_required_field", 1)
	assertSummaryReasonCount(t, summary.Rows(), "clash_proxy_group_ignored", 1)
}

func TestParseNodesDeduplicatesEquivalentNodes(t *testing.T) {
	nodes, summary, err := ParseNodes([]byte(`
[
  {"type":"http","tag":"one","server":"127.0.0.1","server_port":18084},
  {"type":"http","tag":"two","server":"127.0.0.1","server_port":18084}
]
`))
	if err != nil {
		t.Fatalf("ParseNodes error = %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("nodes = %#v, want one deduplicated node", nodes)
	}
	assertSummaryReasonCount(t, summary.Rows(), "duplicate_node", 1)
}

func assertSummaryReasonCount(t *testing.T, rows []SkippedEntrySummary, reason string, want int) {
	t.Helper()
	for _, row := range rows {
		if row.Reason == reason {
			if row.Count != want {
				t.Fatalf("summary[%s] count = %d, want %d; summary=%#v", reason, row.Count, want, rows)
			}
			return
		}
	}
	t.Fatalf("summary missing reason %s: %#v", reason, rows)
}
