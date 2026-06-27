package subscriptions

import (
	"encoding/json"
	"errors"
	"strings"
)

var ErrManualImportRequired = errors.New("manual node import required")

func ParseManualNodeImport(importText string) ([]ParsedNode, SkippedEntrySummarySet, error) {
	text := strings.TrimSpace(importText)
	if text == "" {
		return nil, SkippedEntrySummarySet{}, ErrManualImportRequired
	}
	if strings.HasPrefix(text, "{") {
		raw := json.RawMessage(text)
		node, reason := ParseSingBoxOutboundNode(raw)
		if reason == "" {
			return []ParsedNode{node}, SkippedEntrySummarySet{}, nil
		}
	}
	return ParseNodes([]byte(text))
}
