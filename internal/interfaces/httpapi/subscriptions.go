package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	appsubscriptions "proxygateway/internal/application/subscriptions"
)

const (
	validationSubscriptionNameSourceRequired = "订阅名称不能为空，类型需为 local 或 remote"
	validationSubscriptionRefreshRequired    = "订阅刷新设置不能为空"
	validationSubscriptionRefreshNonNegative = "订阅刷新间隔不能为负数"
)

type SubscriptionsHandler struct {
	Auth   AuthFunc
	Repo   appsubscriptions.Repository
	Create func(SubscriptionCreateRequest) (any, error)
}

type SubscriptionSubroutesHandler struct {
	Auth              AuthFunc
	Repo              appsubscriptions.Repository
	UpdateAutoRefresh func(subscriptionID string, enabled *bool, intervalSeconds *int) error
	Delete            func(subscriptionID string) error
	Refresh           func(subscriptionID string) (any, error)
}

type SubscriptionCreateRequest struct {
	Name                       string `json:"name"`
	SourceType                 string `json:"source_type"`
	URL                        string `json:"url"`
	Content                    string `json:"content"`
	AutoRefreshEnabled         *bool  `json:"auto_refresh_enabled"`
	AutoRefreshIntervalSeconds int    `json:"auto_refresh_interval_seconds"`
}

type subscriptionAutoRefreshPatchRequest struct {
	AutoRefreshEnabled         *bool `json:"auto_refresh_enabled"`
	AutoRefreshIntervalSeconds *int  `json:"auto_refresh_interval_seconds"`
}

type subscriptionSkippedEntrySummary struct {
	Reason  string                           `json:"reason"`
	Count   int                              `json:"count"`
	Message string                           `json:"message"`
	Details []subscriptionSkippedEntryDetail `json:"details,omitempty"`
}

type subscriptionSkippedEntryDetail struct {
	Name      string `json:"name,omitempty"`
	EntryType string `json:"entry_type,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

func (h SubscriptionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req SubscriptionCreateRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.SourceType = strings.TrimSpace(req.SourceType)
		if req.Name == "" || (req.SourceType != "local" && req.SourceType != "remote") {
			writeError(w, http.StatusBadRequest, validationSubscriptionNameSourceRequired)
			return
		}
		if req.AutoRefreshIntervalSeconds < 0 {
			writeError(w, http.StatusBadRequest, validationSubscriptionRefreshNonNegative)
			return
		}
		result, err := h.Create(req)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodGet:
		page, pageSize := parsePagination(r)
		offset := (page - 1) * pageSize
		list, err := h.Repo.List(r.Context(), appsubscriptions.ListFilter{Limit: pageSize, Offset: offset})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list subscriptions")
			return
		}
		items := make([]map[string]any, 0, len(list.Items))
		for _, record := range list.Items {
			items = append(items, subscriptionListItem(record))
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "subscriptions": items, "total": list.Total, "page": page, "page_size": pageSize})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h SubscriptionSubroutesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	subscriptionID, rest := subscriptionSubroute(r.URL.Path)
	if subscriptionID == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if rest == "" {
		h.handleSubscriptionRoot(w, r, subscriptionID)
		return
	}
	if rest != "refresh" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := h.Refresh(subscriptionID)
	if err != nil {
		writeStatusError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h SubscriptionSubroutesHandler) handleSubscriptionRoot(w http.ResponseWriter, r *http.Request, subscriptionID string) {
	switch r.Method {
	case http.MethodGet:
		record, found, err := h.Repo.Load(contextOrBackground(r), subscriptionID)
		if err != nil || !found {
			writeError(w, http.StatusNotFound, "subscription not found")
			return
		}
		body := subscriptionDetail(record)
		writeJSON(w, http.StatusOK, body)
	case http.MethodPatch:
		var req subscriptionAutoRefreshPatchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.AutoRefreshEnabled == nil && req.AutoRefreshIntervalSeconds == nil {
			writeError(w, http.StatusBadRequest, validationSubscriptionRefreshRequired)
			return
		}
		if req.AutoRefreshIntervalSeconds != nil && *req.AutoRefreshIntervalSeconds < 0 {
			writeError(w, http.StatusBadRequest, validationSubscriptionRefreshNonNegative)
			return
		}
		if err := h.UpdateAutoRefresh(subscriptionID, req.AutoRefreshEnabled, req.AutoRefreshIntervalSeconds); err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
	case http.MethodDelete:
		if err := h.Delete(subscriptionID); err != nil {
			writeError(w, http.StatusNotFound, "subscription not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func subscriptionSubroute(path string) (string, string) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/subscriptions/"), "/")
	if trimmed == "" {
		return "", ""
	}
	id, rest, ok := strings.Cut(trimmed, "/")
	if !ok {
		return id, ""
	}
	return id, strings.Trim(rest, "/")
}

func subscriptionListItem(record appsubscriptions.Record) map[string]any {
	lastRefreshAt := any(nil)
	if record.UpdatedAt > 0 {
		lastRefreshAt = record.UpdatedAt
	}
	return map[string]any{
		"id":                            record.ID,
		"name":                          record.Name,
		"source_type":                   record.SourceType,
		"state":                         subscriptionState(record),
		"node_count":                    record.ImportedNodes,
		"skipped_count":                 record.SkippedEntries,
		"skipped_entries":               record.SkippedEntries,
		"skipped_entry_summary":         skippedSubscriptionSummaryRows(record.SkippedSummaryJSON),
		"last_error":                    record.LastError,
		"last_refresh_at":               lastRefreshAt,
		"refresh_policy":                subscriptionRefreshPolicy(record.AutoRefreshEnabled, record.AutoRefreshIntervalSeconds),
		"auto_refresh_enabled":          record.AutoRefreshEnabled,
		"auto_refresh_interval_seconds": record.AutoRefreshIntervalSeconds,
	}
}

func subscriptionDetail(record appsubscriptions.Record) map[string]any {
	body := map[string]any{
		"id":                    record.ID,
		"name":                  record.Name,
		"source_type":           record.SourceType,
		"state":                 subscriptionState(record),
		"url":                   record.URL,
		"node_count":            record.ImportedNodes,
		"skipped_count":         record.SkippedEntries,
		"skipped_entry_summary": skippedSubscriptionSummaryRows(record.SkippedSummaryJSON),
		"last_error":            record.LastError,
		"last_refresh_at":       nil,
		"refresh_policy":        subscriptionRefreshPolicy(record.AutoRefreshEnabled, record.AutoRefreshIntervalSeconds),
	}
	if record.SourceType != "remote" {
		body["content"] = record.Content
	}
	return body
}

func subscriptionState(record appsubscriptions.Record) string {
	if record.LastError != "" {
		return "error"
	}
	if !record.AutoRefreshEnabled {
		return "disabled"
	}
	return "active"
}

func subscriptionRefreshPolicy(enabled bool, intervalSeconds int) map[string]any {
	if !enabled {
		return map[string]any{"mode": "disabled", "interval_seconds": nil}
	}
	if intervalSeconds > 0 {
		return map[string]any{"mode": "custom", "interval_seconds": intervalSeconds}
	}
	return map[string]any{"mode": "inherit", "interval_seconds": nil}
}

func skippedSubscriptionSummaryRows(text string) []subscriptionSkippedEntrySummary {
	var rows []subscriptionSkippedEntrySummary
	if err := json.Unmarshal([]byte(text), &rows); err != nil {
		return nil
	}
	return rows
}

func contextOrBackground(r *http.Request) context.Context {
	if r == nil {
		return context.Background()
	}
	return r.Context()
}
