package subscriptions

import "encoding/json"

type ParsedImportContent struct {
	Nodes              []ParsedNode
	SkippedEntries     int
	SkippedSummary     []SkippedEntrySummary
	SkippedSummaryJSON string
}

func ParseImportContent(content string) (ParsedImportContent, error) {
	nodes, skippedSummary, err := ParseNodes([]byte(content))
	if err != nil {
		return ParsedImportContent{}, err
	}
	rows := skippedSummary.Rows()
	raw, err := json.Marshal(rows)
	if err != nil {
		return ParsedImportContent{}, err
	}
	return ParsedImportContent{
		Nodes:              nodes,
		SkippedEntries:     skippedSummary.Count(),
		SkippedSummary:     rows,
		SkippedSummaryJSON: string(raw),
	}, nil
}
