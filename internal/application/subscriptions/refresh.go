package subscriptions

const (
	skipReasonUnsupportedFunctionalOutbound = "unsupported_functional_outbound"
	skipReasonClashProxyGroupIgnored        = "clash_proxy_group_ignored"
)

type SkippedEntrySummary struct {
	Reason  string `json:"reason"`
	Count   int    `json:"count"`
	Message string `json:"message"`
}

type StickyProfileEvaluationRef struct {
	ID            string
	Name          string
	ConfigVersion int64
}

type RefreshImportResult struct {
	SubscriptionID           string
	ImportedNodes            int
	SkippedEntrySummary      []SkippedEntrySummary
	StickyProfilesToEvaluate []StickyProfileEvaluationRef
}

type RefreshSuccessOutcome struct {
	SubscriptionID           string
	Result                   string
	ReasonCode               string
	ImportedCount            int
	IgnoredCount             int
	SkippedCount             int
	IgnoredSummary           []SkippedEntrySummary
	SkippedSummary           []SkippedEntrySummary
	EnqueueObservation       bool
	StickyProfilesToEvaluate []StickyProfileEvaluationRef
}

type RefreshFailureOutcome struct {
	SubscriptionID   string
	ReasonCode       string
	PersistLastError bool
}

func BuildRefreshSuccessOutcome(result RefreshImportResult) RefreshSuccessOutcome {
	ignoredCount, ignoredSummary, skippedCount, skippedSummary := splitSkippedSummaries(result.SkippedEntrySummary)
	runResult := "success"
	reasonCode := "completed"
	if result.ImportedNodes == 0 {
		runResult = "warning"
		reasonCode = "no_importable_nodes"
	}
	return RefreshSuccessOutcome{
		SubscriptionID:           result.SubscriptionID,
		Result:                   runResult,
		ReasonCode:               reasonCode,
		ImportedCount:            result.ImportedNodes,
		IgnoredCount:             ignoredCount,
		SkippedCount:             skippedCount,
		IgnoredSummary:           ignoredSummary,
		SkippedSummary:           skippedSummary,
		EnqueueObservation:       true,
		StickyProfilesToEvaluate: result.StickyProfilesToEvaluate,
	}
}

func BuildRefreshFetchFailure(subscriptionID string) RefreshFailureOutcome {
	return RefreshFailureOutcome{
		SubscriptionID:   subscriptionID,
		ReasonCode:       "fetch_failed",
		PersistLastError: true,
	}
}

func BuildRefreshImportFailure(subscriptionID string, invalidContent bool) RefreshFailureOutcome {
	reasonCode := "import_failed"
	if invalidContent {
		reasonCode = "parse_failed"
	}
	return RefreshFailureOutcome{
		SubscriptionID:   subscriptionID,
		ReasonCode:       reasonCode,
		PersistLastError: true,
	}
}

func splitSkippedSummaries(rows []SkippedEntrySummary) (int, []SkippedEntrySummary, int, []SkippedEntrySummary) {
	ignoredRows := []SkippedEntrySummary{}
	skippedRows := []SkippedEntrySummary{}
	ignoredCount := 0
	skippedCount := 0
	for _, row := range rows {
		if row.Reason == skipReasonClashProxyGroupIgnored || row.Reason == skipReasonUnsupportedFunctionalOutbound {
			ignoredRows = append(ignoredRows, row)
			ignoredCount += row.Count
			continue
		}
		skippedRows = append(skippedRows, row)
		skippedCount += row.Count
	}
	return ignoredCount, ignoredRows, skippedCount, skippedRows
}
