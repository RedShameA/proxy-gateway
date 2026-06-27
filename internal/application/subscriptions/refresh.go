package subscriptions

import "sort"

const (
	skipReasonUnsupportedFunctionalOutbound = "unsupported_functional_outbound"
	skipReasonMissingRequiredField          = "missing_required_field"
	skipReasonMalformedEntry                = "malformed_entry"
	skipReasonUnsupportedNodeType           = "unsupported_node_type"
	skipReasonUnsupportedOption             = "unsupported_option"
	skipReasonClashProxyGroupIgnored        = "clash_proxy_group_ignored"
	skipReasonDuplicateNode                 = "duplicate_node"
)

type SkippedEntrySummary struct {
	Reason  string               `json:"reason"`
	Count   int                  `json:"count"`
	Message string               `json:"message"`
	Details []SkippedEntryDetail `json:"details,omitempty"`
}

type SkippedEntryDetail struct {
	Name      string `json:"name,omitempty"`
	EntryType string `json:"entry_type,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type SkippedEntrySummarySet map[string][]SkippedEntryDetail

func (s SkippedEntrySummarySet) add(reason string) {
	s.addDetail(reason, SkippedEntryDetail{})
}

func (s SkippedEntrySummarySet) addDetail(reason string, detail SkippedEntryDetail) {
	if reason == "" {
		return
	}
	s[reason] = append(s[reason], detail)
}

func (s SkippedEntrySummarySet) count() int {
	total := 0
	for _, details := range s {
		total += len(details)
	}
	return total
}

func (s SkippedEntrySummarySet) Count() int {
	return s.count()
}

func (s SkippedEntrySummarySet) rows() []SkippedEntrySummary {
	reasons := make([]string, 0, len(s))
	for reason := range s {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	rows := make([]SkippedEntrySummary, 0, len(reasons))
	for _, reason := range reasons {
		details := nonEmptySkippedDetails(s[reason])
		rows = append(rows, SkippedEntrySummary{
			Reason:  reason,
			Count:   len(s[reason]),
			Message: skippedReasonMessage(reason),
			Details: details,
		})
	}
	return rows
}

func (s SkippedEntrySummarySet) Rows() []SkippedEntrySummary {
	return s.rows()
}

func nonEmptySkippedDetails(details []SkippedEntryDetail) []SkippedEntryDetail {
	filtered := make([]SkippedEntryDetail, 0, len(details))
	for _, detail := range details {
		if detail.Name == "" && detail.EntryType == "" && detail.Detail == "" {
			continue
		}
		filtered = append(filtered, detail)
	}
	return filtered
}

func skippedReasonMessage(reason string) string {
	switch reason {
	case skipReasonUnsupportedFunctionalOutbound:
		return "功能出站：已跳过，因为它们不是可拨号 Nodes"
	case skipReasonMissingRequiredField:
		return "缺少必填字段：已跳过，需要补齐服务器、端口或协议认证字段"
	case skipReasonMalformedEntry:
		return "格式错误：已跳过，条目不是有效的代理配置"
	case skipReasonUnsupportedNodeType:
		return "协议不支持：已跳过，当前 sing-box 拨号能力暂不支持该类型"
	case skipReasonUnsupportedOption:
		return "配置选项不支持：已跳过，当前协议引擎无法按该配置拨号"
	case skipReasonClashProxyGroupIgnored:
		return "策略组：已跳过，因为它们不是可拨号 Nodes"
	case skipReasonDuplicateNode:
		return "重复节点：已跳过，因为同一个节点已在本次导入中出现"
	default:
		return reason
	}
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

func (outcome RefreshFailureOutcome) MaintenanceDetail(detail map[string]any) map[string]any {
	detail = copyMaintenanceDetail(detail)
	detail["subscription_id"] = outcome.SubscriptionID
	return detail
}

func (outcome RefreshSuccessOutcome) MaintenanceDetail(detail map[string]any) map[string]any {
	detail = copyMaintenanceDetail(detail)
	detail["subscription_id"] = outcome.SubscriptionID
	detail["imported_count"] = outcome.ImportedCount
	detail["imported"] = outcome.ImportedCount
	detail["added_count"] = 0
	detail["updated_count"] = 0
	detail["retained_count"] = outcome.ImportedCount
	detail["removed_count"] = 0
	detail["ignored_count"] = outcome.IgnoredCount
	detail["skipped_count"] = outcome.SkippedCount
	detail["ignored_entry_summary"] = outcome.IgnoredSummary
	detail["skipped_entry_summary"] = outcome.SkippedSummary
	return detail
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

func copyMaintenanceDetail(detail map[string]any) map[string]any {
	if detail == nil {
		return map[string]any{}
	}
	copied := make(map[string]any, len(detail))
	for key, value := range detail {
		copied[key] = value
	}
	return copied
}
