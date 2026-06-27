package subscriptions

import "testing"

func TestImportResultFromRecordParsesSkippedSummary(t *testing.T) {
	result := ImportResultFromRecord(ImportResultRecord{
		ID:                 "sub_1",
		ImportedNodes:      2,
		SkippedEntries:     1,
		SkippedSummaryJSON: `[{"reason":"missing_required_field","count":1,"message":"missing"}]`,
	})

	if result.ID != "sub_1" || result.ImportedNodes != 2 || result.SkippedEntries != 1 {
		t.Fatalf("result counts = %#v", result)
	}
	if len(result.SkippedEntrySummary) != 1 || result.SkippedEntrySummary[0].Reason != "missing_required_field" {
		t.Fatalf("summary = %#v", result.SkippedEntrySummary)
	}
	if result.StickyProfilesToEvaluate != nil {
		t.Fatalf("sticky refs = %#v, want nil", result.StickyProfilesToEvaluate)
	}
}

func TestSkippedSummaryRowsFromJSONReturnsNilForInvalidJSON(t *testing.T) {
	if rows := SkippedSummaryRowsFromJSON(`not-json`); rows != nil {
		t.Fatalf("rows = %#v, want nil", rows)
	}
}

func TestRefreshImportResultFromImportResultPreservesCurrentSummaryShape(t *testing.T) {
	result := NewImportResult(
		"sub_1",
		0,
		1,
		[]SkippedEntrySummary{
			{
				Reason:  "missing_required_field",
				Count:   1,
				Message: "missing",
				Details: []SkippedEntryDetail{{Name: "raw-node"}},
			},
		},
		[]StickyProfileEvaluationRef{{ID: "profile_1", Name: "work", ConfigVersion: 3}},
	)

	refresh := RefreshImportResultFromImportResult(result)

	if refresh.SubscriptionID != "sub_1" || refresh.ImportedNodes != 0 {
		t.Fatalf("refresh result = %#v", refresh)
	}
	if len(refresh.SkippedEntrySummary) != 1 || refresh.SkippedEntrySummary[0].Reason != "missing_required_field" {
		t.Fatalf("refresh summary = %#v", refresh.SkippedEntrySummary)
	}
	if refresh.SkippedEntrySummary[0].Details != nil {
		t.Fatalf("refresh summary details = %#v, want nil", refresh.SkippedEntrySummary[0].Details)
	}
	if len(refresh.StickyProfilesToEvaluate) != 1 || refresh.StickyProfilesToEvaluate[0].ID != "profile_1" {
		t.Fatalf("sticky refs = %#v", refresh.StickyProfilesToEvaluate)
	}
}
