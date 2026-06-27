package subscriptions

import (
	"encoding/json"
	"testing"
)

func TestParseImportContentBuildsSkippedSummaryJSON(t *testing.T) {
	content := `
proxies:
  - name: ok
    type: http
    server: 127.0.0.1
    port: 18080
  - name: missing
    type: http
    port: 18081
`

	parsed, err := ParseImportContent(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Nodes) != 1 || parsed.Nodes[0].Name != "ok" {
		t.Fatalf("nodes = %#v", parsed.Nodes)
	}
	if parsed.SkippedEntries != 1 || len(parsed.SkippedSummary) != 1 {
		t.Fatalf("skipped = %d / %#v", parsed.SkippedEntries, parsed.SkippedSummary)
	}
	var rows []SkippedEntrySummary
	if err := json.Unmarshal([]byte(parsed.SkippedSummaryJSON), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Reason != "missing_required_field" {
		t.Fatalf("summary json rows = %#v", rows)
	}
}
