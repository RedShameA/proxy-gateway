package subscriptions

import "encoding/json"

type ImportResult struct {
	ID                       string                       `json:"id"`
	ImportedNodes            int                          `json:"imported_nodes"`
	SkippedEntries           int                          `json:"skipped_entries"`
	SkippedEntrySummary      []SkippedEntrySummary        `json:"skipped_entry_summary"`
	StickyProfilesToEvaluate []StickyProfileEvaluationRef `json:"-"`
}

func NewImportResult(subscriptionID string, importedNodes, skippedEntries int, skippedSummary []SkippedEntrySummary, stickyRefs []StickyProfileEvaluationRef) ImportResult {
	return ImportResult{
		ID:                       subscriptionID,
		ImportedNodes:            importedNodes,
		SkippedEntries:           skippedEntries,
		SkippedEntrySummary:      skippedSummary,
		StickyProfilesToEvaluate: stickyRefs,
	}
}

func ImportResultFromRecord(record ImportResultRecord) ImportResult {
	return NewImportResult(
		record.ID,
		record.ImportedNodes,
		record.SkippedEntries,
		SkippedSummaryRowsFromJSON(record.SkippedSummaryJSON),
		nil,
	)
}

func RefreshImportResultFromImportResult(result ImportResult) RefreshImportResult {
	out := RefreshImportResult{
		SubscriptionID:           result.ID,
		ImportedNodes:            result.ImportedNodes,
		SkippedEntrySummary:      make([]SkippedEntrySummary, 0, len(result.SkippedEntrySummary)),
		StickyProfilesToEvaluate: result.StickyProfilesToEvaluate,
	}
	for _, row := range result.SkippedEntrySummary {
		out.SkippedEntrySummary = append(out.SkippedEntrySummary, SkippedEntrySummary{
			Reason:  row.Reason,
			Count:   row.Count,
			Message: row.Message,
		})
	}
	return out
}

func SkippedSummaryRowsFromJSON(text string) []SkippedEntrySummary {
	var rows []SkippedEntrySummary
	if err := json.Unmarshal([]byte(text), &rows); err != nil {
		return nil
	}
	return rows
}
