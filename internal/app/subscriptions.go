package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"sort"
	"strings"

	appsubscriptions "proxygateway/internal/application/subscriptions"
)

const (
	skipReasonUnsupportedFunctionalOutbound = "unsupported_functional_outbound"
	skipReasonMissingRequiredField          = "missing_required_field"
	skipReasonMalformedEntry                = "malformed_entry"
	skipReasonUnsupportedNodeType           = "unsupported_node_type"
	skipReasonUnsupportedOption             = "unsupported_option"
	skipReasonClashProxyGroupIgnored        = "clash_proxy_group_ignored"
	skipReasonDuplicateNode                 = "duplicate_node"
)

type skippedEntrySummary struct {
	Reason  string               `json:"reason"`
	Count   int                  `json:"count"`
	Message string               `json:"message"`
	Details []skippedEntryDetail `json:"details,omitempty"`
}

type skippedEntryDetail struct {
	Name      string `json:"name,omitempty"`
	EntryType string `json:"entry_type,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type skippedEntrySummarySet map[string][]skippedEntryDetail

type subscriptionRecord struct {
	ID                         string
	Name                       string
	SourceType                 string
	URL                        string
	Content                    string
	AutoRefreshEnabled         bool
	AutoRefreshIntervalSeconds int
}

type subscriptionImportResult struct {
	ID                  string                `json:"id"`
	ImportedNodes       int                   `json:"imported_nodes"`
	SkippedEntries      int                   `json:"skipped_entries"`
	SkippedEntrySummary []skippedEntrySummary `json:"skipped_entry_summary"`

	stickyProfilesToEvaluate []stickyProfileEvaluationRef
}

type stickyProfileEvaluationRef struct {
	ID            string
	Name          string
	ConfigVersion int64
}

func (s skippedEntrySummarySet) add(reason string) {
	s.addDetail(reason, skippedEntryDetail{})
}

func (s skippedEntrySummarySet) addN(reason string, count int) {
	if count <= 0 {
		return
	}
	for range count {
		s.add(reason)
	}
}

func (s skippedEntrySummarySet) addDetail(reason string, detail skippedEntryDetail) {
	if reason == "" {
		return
	}
	s[reason] = append(s[reason], detail)
}

func (s skippedEntrySummarySet) count() int {
	total := 0
	for _, details := range s {
		total += len(details)
	}
	return total
}

func (s skippedEntrySummarySet) rows() []skippedEntrySummary {
	reasons := make([]string, 0, len(s))
	for reason := range s {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	rows := make([]skippedEntrySummary, 0, len(reasons))
	for _, reason := range reasons {
		details := nonEmptySkippedDetails(s[reason])
		rows = append(rows, skippedEntrySummary{
			Reason:  reason,
			Count:   len(s[reason]),
			Message: skippedReasonMessage(reason),
			Details: details,
		})
	}
	return rows
}

func nonEmptySkippedDetails(details []skippedEntryDetail) []skippedEntryDetail {
	filtered := make([]skippedEntryDetail, 0, len(details))
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

func truncateSkippedDetail(text string) string {
	text = strings.TrimSpace(text)
	const maxDetailLength = 160
	if len(text) <= maxDetailLength {
		return text
	}
	return text[:maxDetailLength] + "..."
}

func (g *Gateway) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name                       string `json:"name"`
			SourceType                 string `json:"source_type"`
			URL                        string `json:"url"`
			Content                    string `json:"content"`
			AutoRefreshEnabled         *bool  `json:"auto_refresh_enabled"`
			AutoRefreshIntervalSeconds int    `json:"auto_refresh_interval_seconds"`
		}
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
		autoRefreshEnabled := true
		if req.AutoRefreshEnabled != nil {
			autoRefreshEnabled = *req.AutoRefreshEnabled
		}
		id, err := prefixedID("sub")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create subscription id")
			return
		}
		sub := subscriptionRecord{
			ID:                         id,
			Name:                       req.Name,
			SourceType:                 req.SourceType,
			URL:                        req.URL,
			Content:                    req.Content,
			AutoRefreshEnabled:         autoRefreshEnabled,
			AutoRefreshIntervalSeconds: req.AutoRefreshIntervalSeconds,
		}
		content, err := g.subscriptionContentForImport(sub)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		result, err := g.createSubscriptionWithContent(sub, content)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errInvalidSubscriptionContent) {
				status = http.StatusBadRequest
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodGet:
		// Pagination
		page, pageSize := parsePagination(r)
		offset := (page - 1) * pageSize

		// Get total count
		var total int
		_ = g.db.QueryRow(`SELECT COUNT(*) FROM subscriptions`).Scan(&total)

		rows, err := g.db.Query(`SELECT id, name, source_type, imported_nodes, skipped_entries, skipped_summary_json, last_error, auto_refresh_enabled, auto_refresh_interval_seconds, updated_at FROM subscriptions ORDER BY created_at, id LIMIT ? OFFSET ?`, pageSize, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list subscriptions")
			return
		}
		defer rows.Close()
		var subs []map[string]any
		for rows.Next() {
			var id, name, sourceType, skippedSummaryJSON, lastError string
			var imported, skipped, autoRefreshEnabled, autoRefreshIntervalSeconds int
			var updatedAt int64
			if err := rows.Scan(&id, &name, &sourceType, &imported, &skipped, &skippedSummaryJSON, &lastError, &autoRefreshEnabled, &autoRefreshIntervalSeconds, &updatedAt); err != nil {
				writeError(w, http.StatusInternalServerError, "scan subscription")
				return
			}
			state := "active"
			if lastError != "" {
				state = "error"
			} else if autoRefreshEnabled != 1 {
				state = "disabled"
			}
			refreshPolicy := map[string]any{"mode": "inherit", "interval_seconds": nil}
			if autoRefreshEnabled != 1 {
				refreshPolicy = map[string]any{"mode": "disabled", "interval_seconds": nil}
			} else if autoRefreshIntervalSeconds > 0 {
				refreshPolicy = map[string]any{"mode": "custom", "interval_seconds": autoRefreshIntervalSeconds}
			}
			var lastRefreshAt any = nil
			if updatedAt > 0 {
				lastRefreshAt = updatedAt
			}
			subs = append(subs, map[string]any{
				"id":                            id,
				"name":                          name,
				"source_type":                   sourceType,
				"state":                         state,
				"node_count":                    imported,
				"skipped_count":                 skipped,
				"skipped_entries":               skipped,
				"skipped_entry_summary":         skippedSummaryRowsFromJSON(skippedSummaryJSON),
				"last_error":                    lastError,
				"last_refresh_at":               lastRefreshAt,
				"refresh_policy":                refreshPolicy,
				"auto_refresh_enabled":          autoRefreshEnabled == 1,
				"auto_refresh_interval_seconds": autoRefreshIntervalSeconds,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": subs, "subscriptions": subs, "total": total, "page": page, "page_size": pageSize})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

var errInvalidSubscriptionContent = errors.New("invalid subscription content")

func (g *Gateway) handleSubscriptionSubroutes(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/subscriptions/")
	subscriptionID, rest, ok := strings.Cut(trimmed, "/")
	if !ok {
		switch r.Method {
		case http.MethodGet:
			var sub subscriptionRecord
			var autoRefreshEnabled int
			var imported, skipped int
			var skippedSummaryJSON, lastError string
			err := g.db.QueryRow(
				`SELECT id, name, source_type, url, content, imported_nodes, skipped_entries, skipped_summary_json, last_error, auto_refresh_enabled, auto_refresh_interval_seconds FROM subscriptions WHERE id = ?`,
				subscriptionID,
			).Scan(&sub.ID, &sub.Name, &sub.SourceType, &sub.URL, &sub.Content, &imported, &skipped, &skippedSummaryJSON, &lastError, &autoRefreshEnabled, &sub.AutoRefreshIntervalSeconds)
			if err != nil {
				writeError(w, http.StatusNotFound, "subscription not found")
				return
			}
			sub.AutoRefreshEnabled = autoRefreshEnabled == 1
			state := "active"
			if lastError != "" {
				state = "error"
			} else if !sub.AutoRefreshEnabled {
				state = "disabled"
			}
			refreshPolicy := map[string]any{"mode": "inherit", "interval_seconds": nil}
			if !sub.AutoRefreshEnabled {
				refreshPolicy = map[string]any{"mode": "disabled", "interval_seconds": nil}
			} else if sub.AutoRefreshIntervalSeconds > 0 {
				refreshPolicy = map[string]any{"mode": "custom", "interval_seconds": sub.AutoRefreshIntervalSeconds}
			}
			body := map[string]any{
				"id":                    sub.ID,
				"name":                  sub.Name,
				"source_type":           sub.SourceType,
				"state":                 state,
				"url":                   sub.URL,
				"node_count":            imported,
				"skipped_count":         skipped,
				"skipped_entry_summary": skippedSummaryRowsFromJSON(skippedSummaryJSON),
				"last_error":            lastError,
				"last_refresh_at":       nil,
				"refresh_policy":        refreshPolicy,
			}
			if sub.SourceType != "remote" {
				body["content"] = sub.Content
			}
			writeJSON(w, http.StatusOK, body)
		case http.MethodPatch:
			var req struct {
				AutoRefreshEnabled         *bool `json:"auto_refresh_enabled"`
				AutoRefreshIntervalSeconds *int  `json:"auto_refresh_interval_seconds"`
			}
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
			if err := g.updateSubscriptionAutoRefresh(subscriptionID, req.AutoRefreshEnabled, req.AutoRefreshIntervalSeconds); err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
		case http.MethodDelete:
			if err := g.deleteSubscriptionSource(subscriptionID); err != nil {
				writeError(w, http.StatusNotFound, "subscription not found")
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
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
	sub, err := g.loadSubscription(subscriptionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	runID, err := g.enqueueSubscriptionRefreshRun(sub.ID, sub.Name, "manual")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create subscription refresh run")
		return
	}
	if err := g.runSubscriptionRefreshMaintenanceRun(runID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errInvalidSubscriptionContent) {
			status = http.StatusBadRequest
		} else if failedRun, loadErr := g.loadMaintenanceRun(runID); loadErr == nil && failedRun.ReasonCode == "fetch_failed" {
			status = http.StatusBadGateway
		}
		writeError(w, status, err.Error())
		return
	}
	result := g.subscriptionImportResult(sub.ID)
	writeJSON(w, http.StatusOK, result)
}

func (g *Gateway) subscriptionImportResult(subscriptionID string) subscriptionImportResult {
	var result subscriptionImportResult
	var skippedSummaryJSON string
	_ = g.db.QueryRow(
		`SELECT id, imported_nodes, skipped_entries, skipped_summary_json
		   FROM subscriptions
		  WHERE id = ?`,
		subscriptionID,
	).Scan(&result.ID, &result.ImportedNodes, &result.SkippedEntries, &skippedSummaryJSON)
	result.SkippedEntrySummary = skippedSummaryRowsFromJSON(skippedSummaryJSON)
	return result
}

func (g *Gateway) deleteSubscriptionSource(subscriptionID string) error {
	tx, err := g.db.Begin()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	result, err := appsubscriptions.DeleteSubscriptionSource(subscriptionDeleteRepositoryTx{tx: tx}, subscriptionID)
	if errors.Is(err, appsubscriptions.ErrSubscriptionNotFound) {
		return errors.New("subscription not found")
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	g.invalidateRuntimeFingerprints(result.DeletedFingerprints)
	g.notifyMaintenanceRunner()
	return nil
}

func (g *Gateway) loadSubscription(id string) (subscriptionRecord, error) {
	var sub subscriptionRecord
	var autoRefreshEnabled int
	err := g.db.QueryRow(
		`SELECT id, name, source_type, url, content, auto_refresh_enabled, auto_refresh_interval_seconds FROM subscriptions WHERE id = ?`,
		id,
	).Scan(&sub.ID, &sub.Name, &sub.SourceType, &sub.URL, &sub.Content, &autoRefreshEnabled, &sub.AutoRefreshIntervalSeconds)
	sub.AutoRefreshEnabled = autoRefreshEnabled == 1
	return sub, err
}

func (g *Gateway) updateSubscriptionAutoRefresh(subscriptionID string, enabled *bool, intervalSeconds *int) error {
	var exists int
	if err := g.db.QueryRow(`SELECT 1 FROM subscriptions WHERE id = ?`, subscriptionID).Scan(&exists); err != nil {
		return errors.New("subscription not found")
	}
	if enabled != nil {
		value := 0
		if *enabled {
			value = 1
		}
		if _, err := g.db.Exec(`UPDATE subscriptions SET auto_refresh_enabled = ?, updated_at = ? WHERE id = ?`, value, unixMillisNow(), subscriptionID); err != nil {
			return err
		}
	}
	if intervalSeconds != nil {
		if _, err := g.db.Exec(`UPDATE subscriptions SET auto_refresh_interval_seconds = ?, updated_at = ? WHERE id = ?`, *intervalSeconds, unixMillisNow(), subscriptionID); err != nil {
			return err
		}
	}
	return nil
}

func (g *Gateway) subscriptionContentForImport(sub subscriptionRecord) (string, error) {
	if sub.SourceType != "remote" {
		return sub.Content, nil
	}
	if strings.TrimSpace(sub.URL) == "" {
		return "", errors.New(validationSubscriptionURLRequired)
	}
	resp, err := externalHTTPGet(sub.URL)
	if err != nil {
		return "", errors.New("fetch subscription")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("fetch subscription status")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return "", errors.New("read subscription")
	}
	return string(raw), nil
}

func (g *Gateway) createSubscriptionWithContent(sub subscriptionRecord, content string) (subscriptionImportResult, error) {
	return g.importSubscriptionWithContent(sub, content, false)
}

func (g *Gateway) refreshSubscriptionWithContent(sub subscriptionRecord, content string) (subscriptionImportResult, error) {
	return g.importSubscriptionWithContent(sub, content, true)
}

func (g *Gateway) importSubscriptionWithContent(sub subscriptionRecord, content string, refresh bool) (subscriptionImportResult, error) {
	nodes, skippedSummary, err := parseSubscriptionNodes([]byte(content))
	if err != nil {
		return subscriptionImportResult{}, errors.Join(errInvalidSubscriptionContent, err)
	}
	skippedSummaryRows := skippedSummary.rows()
	skippedSummaryJSON, err := json.Marshal(skippedSummaryRows)
	if err != nil {
		return subscriptionImportResult{}, err
	}
	skipped := skippedSummary.count()
	tx, err := g.db.Begin()
	if err != nil {
		return subscriptionImportResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	now := unixMillisNow()
	if refresh {
		_, err = tx.Exec(
			`UPDATE subscriptions
			 SET content = ?, imported_nodes = ?, skipped_entries = ?, skipped_summary_json = ?, last_error = '', updated_at = ?
			 WHERE id = ?`,
			content, len(nodes), skipped, string(skippedSummaryJSON), now, sub.ID,
		)
	} else {
		_, err = tx.Exec(
			`INSERT INTO subscriptions (id, name, source_type, url, content, imported_nodes, skipped_entries, skipped_summary_json, auto_refresh_enabled, auto_refresh_interval_seconds, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sub.ID, sub.Name, sub.SourceType, sub.URL, content, len(nodes), skipped, string(skippedSummaryJSON), boolInt(sub.AutoRefreshEnabled), sub.AutoRefreshIntervalSeconds, now, now,
		)
	}
	if err != nil {
		return subscriptionImportResult{}, err
	}
	currentNodeIDs := make([]string, 0, len(nodes))
	for _, n := range nodes {
		nodeID, err := g.upsertNodeTx(tx, n, sub.ID, sub.Name, "subscription")
		if err != nil {
			return subscriptionImportResult{}, err
		}
		currentNodeIDs = append(currentNodeIDs, nodeID)
	}
	var deletedFingerprints []string
	var stickyProfilesToEvaluate []stickyProfileEvaluationRef
	if refresh {
		pruneResult, err := appsubscriptions.PruneRefreshSnapshot(subscriptionRefreshSnapshotRepositoryTx{tx: tx}, sub.ID, currentNodeIDs)
		if err != nil {
			return subscriptionImportResult{}, err
		}
		deletedFingerprints = pruneResult.DeletedFingerprints
		stickyProfilesToEvaluate = toStickyProfileEvaluationRefs(pruneResult.StickyProfilesToEvaluate)
	}
	if err := tx.Commit(); err != nil {
		return subscriptionImportResult{}, err
	}
	committed = true
	g.invalidateRuntimeFingerprints(deletedFingerprints)
	return subscriptionImportResult{
		ID:                       sub.ID,
		ImportedNodes:            len(nodes),
		SkippedEntries:           skipped,
		SkippedEntrySummary:      skippedSummaryRows,
		stickyProfilesToEvaluate: stickyProfilesToEvaluate,
	}, nil
}

type subscriptionRefreshSnapshotRepositoryTx struct {
	tx *sql.Tx
}

func (r subscriptionRefreshSnapshotRepositoryTx) ExistingSourceNodeIDs(subscriptionID string) ([]string, error) {
	return nodeIDsForSourceTx(r.tx, subscriptionID, "subscription")
}

func (r subscriptionRefreshSnapshotRepositoryTx) DeleteSubscriptionNodeSource(nodeID, subscriptionID string) error {
	_, err := r.tx.Exec(`DELETE FROM node_sources WHERE node_id = ? AND source_id = ? AND source_type = 'subscription'`, nodeID, subscriptionID)
	return err
}

func (r subscriptionRefreshSnapshotRepositoryTx) RetainStickyProfilesForRemovedNode(nodeID string) ([]appsubscriptions.StickyProfileEvaluationRef, error) {
	refs, err := retainStickyProfilesForRemovedSubscriptionNodeTx(r.tx, nodeID)
	if err != nil {
		return nil, err
	}
	return toApplicationStickyProfileEvaluationRefs(refs), nil
}

func (r subscriptionRefreshSnapshotRepositoryTx) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	return cleanupNodesWithoutReferencesTx(r.tx, nodeIDs)
}

func (r subscriptionRefreshSnapshotRepositoryTx) RetainedStickyProfilesForRefresh() ([]appsubscriptions.StickyProfileEvaluationRef, error) {
	refs, err := retainedStickyProfilesForSubscriptionRefreshTx(r.tx)
	if err != nil {
		return nil, err
	}
	return toApplicationStickyProfileEvaluationRefs(refs), nil
}

func nodeIDsForSourceTx(tx *sql.Tx, sourceID, sourceType string) ([]string, error) {
	rows, err := tx.Query(`SELECT node_id FROM node_sources WHERE source_id = ? AND source_type = ?`, sourceID, sourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodeIDs []string
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err != nil {
			return nil, err
		}
		nodeIDs = append(nodeIDs, nodeID)
	}
	return nodeIDs, rows.Err()
}

func cleanupNodesWithoutSourcesTx(tx *sql.Tx, nodeIDs []string) ([]string, error) {
	return cleanupNodesWithoutReferencesTx(tx, nodeIDs)
}

type subscriptionDeleteRepositoryTx struct {
	tx *sql.Tx
}

func (r subscriptionDeleteRepositoryTx) DeleteSubscription(subscriptionID string) (int64, error) {
	res, err := r.tx.Exec(`DELETE FROM subscriptions WHERE id = ?`, subscriptionID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r subscriptionDeleteRepositoryTx) NodeIDsForSource(subscriptionID, sourceType string) ([]string, error) {
	return nodeIDsForSourceTx(r.tx, subscriptionID, sourceType)
}

func (r subscriptionDeleteRepositoryTx) DeleteSubscriptionNodeSources(subscriptionID string) error {
	_, err := r.tx.Exec(`DELETE FROM node_sources WHERE source_id = ? AND source_type = 'subscription'`, subscriptionID)
	return err
}

func (r subscriptionDeleteRepositoryTx) InvalidateProfilesForDeletedSubscription(subscriptionID string) error {
	return invalidateProfilesForDeletedSubscriptionTx(r.tx, subscriptionID)
}

func (r subscriptionDeleteRepositoryTx) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	return cleanupNodesWithoutReferencesTx(r.tx, nodeIDs)
}

func cleanupNodesWithoutReferencesTx(tx *sql.Tx, nodeIDs []string) ([]string, error) {
	var deletedFingerprints []string
	for _, nodeID := range nodeIDs {
		var remainingSources int
		if err := tx.QueryRow(`SELECT count(*) FROM node_sources WHERE node_id = ?`, nodeID).Scan(&remainingSources); err != nil {
			return nil, err
		}
		if remainingSources > 0 {
			continue
		}
		var retainedProfiles int
		if err := tx.QueryRow(`SELECT count(*) FROM retained_profile_nodes WHERE node_id = ?`, nodeID).Scan(&retainedProfiles); err != nil {
			return nil, err
		}
		if retainedProfiles > 0 {
			continue
		}
		fingerprint, err := nodeRuntimeFingerprintTx(tx, nodeID)
		if err != nil {
			if errors.Is(err, errNodeNotFound) {
				continue
			}
			return nil, err
		}
		if _, err := tx.Exec(`DELETE FROM node_observations WHERE node_id = ?`, nodeID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`DELETE FROM nodes WHERE id = ?`, nodeID); err != nil {
			return nil, err
		}
		deletedFingerprints = append(deletedFingerprints, fingerprint)
		if _, err := tx.Exec(
			`UPDATE access_profiles
			    SET current_node_id = '',
			        state = 'invalid_config',
			        last_error = 'referenced node no longer exists',
			        switch_reason = 'missing_fixed_node'
			  WHERE fixed_node_id = ?`,
			nodeID,
		); err != nil {
			return nil, err
		}
		if err := resetDynamicProfilesForRemovedCurrentNodeTx(tx, nodeID); err != nil {
			return nil, err
		}
	}
	return deletedFingerprints, nil
}

func retainStickyProfilesForRemovedSubscriptionNodeTx(tx *sql.Tx, nodeID string) ([]stickyProfileEvaluationRef, error) {
	rows, err := tx.Query(
		`SELECT id, name, config_version
		   FROM access_profiles
		  WHERE node_sticky_enabled = 1
		    AND type IN ('fastest', 'chain')
		    AND state != 'invalid_config'
		    AND (current_node_id = ? OR current_exit_node_id = ?)`,
		nodeID,
		nodeID,
	)
	if err != nil {
		return nil, err
	}
	var profiles []stickyProfileEvaluationRef
	for rows.Next() {
		var profile stickyProfileEvaluationRef
		if err := rows.Scan(&profile.ID, &profile.Name, &profile.ConfigVersion); err != nil {
			_ = rows.Close()
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	now := unixMillisNow()
	for _, profile := range profiles {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO retained_profile_nodes (profile_id, node_id, created_at) VALUES (?, ?, ?)`,
			profile.ID,
			nodeID,
			now,
		); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(
			`UPDATE access_profiles
			    SET state = 'degraded',
			        last_error = 'current node no longer exists',
			        switch_reason = 'current_node_removed',
			        last_evaluation_started_at = ?
			  WHERE id = ?`,
			now,
			profile.ID,
		); err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

func toApplicationStickyProfileEvaluationRefs(refs []stickyProfileEvaluationRef) []appsubscriptions.StickyProfileEvaluationRef {
	out := make([]appsubscriptions.StickyProfileEvaluationRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, appsubscriptions.StickyProfileEvaluationRef{
			ID:            ref.ID,
			Name:          ref.Name,
			ConfigVersion: ref.ConfigVersion,
		})
	}
	return out
}

func retainedStickyProfilesForSubscriptionRefreshTx(tx *sql.Tx) ([]stickyProfileEvaluationRef, error) {
	rows, err := tx.Query(
		`SELECT DISTINCT p.id, p.name, p.config_version
		   FROM access_profiles p
		   JOIN retained_profile_nodes r ON r.profile_id = p.id
		  WHERE p.node_sticky_enabled = 1
		    AND p.type IN ('fastest', 'chain')
		    AND p.state != 'invalid_config'
		  ORDER BY p.created_at, p.id`,
	)
	if err != nil {
		return nil, err
	}
	var profiles []stickyProfileEvaluationRef
	for rows.Next() {
		var profile stickyProfileEvaluationRef
		if err := rows.Scan(&profile.ID, &profile.Name, &profile.ConfigVersion); err != nil {
			_ = rows.Close()
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	now := unixMillisNow()
	for _, profile := range profiles {
		if _, err := tx.Exec(
			`UPDATE access_profiles
			    SET state = 'degraded',
			        last_error = 'current node no longer exists',
			        switch_reason = 'current_node_removed',
			        last_evaluation_started_at = ?
			  WHERE id = ?`,
			now,
			profile.ID,
		); err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

func (g *Gateway) enqueueStickyProfileEvaluationsForRemovedNodes(refs []stickyProfileEvaluationRef) {
	for _, ref := range refs {
		_, _ = g.enqueueProfileEvaluationRun(ref.ID, ref.Name, "current_node_removed", ref.ConfigVersion, true)
	}
}

func retainedNodeIDsForProfileTx(tx *sql.Tx, profileID string) ([]string, error) {
	rows, err := tx.Query(`SELECT node_id FROM retained_profile_nodes WHERE profile_id = ?`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodeIDs []string
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err != nil {
			return nil, err
		}
		nodeIDs = append(nodeIDs, nodeID)
	}
	return nodeIDs, rows.Err()
}

func releaseRetainedProfileNodesExceptTx(tx *sql.Tx, profileID string, keepNodeIDs []string) ([]string, error) {
	retainedNodeIDs, err := retainedNodeIDsForProfileTx(tx, profileID)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, nodeID := range keepNodeIDs {
		if strings.TrimSpace(nodeID) != "" {
			keep[nodeID] = true
		}
	}
	var releaseNodeIDs []string
	for _, nodeID := range retainedNodeIDs {
		if keep[nodeID] {
			continue
		}
		releaseNodeIDs = append(releaseNodeIDs, nodeID)
	}
	for _, nodeID := range releaseNodeIDs {
		if _, err := tx.Exec(`DELETE FROM retained_profile_nodes WHERE profile_id = ? AND node_id = ?`, profileID, nodeID); err != nil {
			return nil, err
		}
	}
	return cleanupNodesWithoutReferencesTx(tx, releaseNodeIDs)
}

func (g *Gateway) releaseRetainedProfileNodes(profileID string, keepNodeIDs []string) ([]string, error) {
	tx, err := g.db.Begin()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	deletedFingerprints, err := releaseRetainedProfileNodesExceptTx(tx, profileID, keepNodeIDs)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return deletedFingerprints, nil
}

func (g *Gateway) profileRetainsNode(profileID, nodeID string) bool {
	if profileID == "" || nodeID == "" {
		return false
	}
	var exists int
	err := g.db.QueryRow(
		`SELECT 1 FROM retained_profile_nodes WHERE profile_id = ? AND node_id = ? LIMIT 1`,
		profileID,
		nodeID,
	).Scan(&exists)
	return err == nil && exists == 1
}

func resetDynamicProfilesForRemovedCurrentNodeTx(tx *sql.Tx, nodeID string) error {
	rows, err := tx.Query(
		`SELECT id, name, type, config_version, auto_evaluation_enabled
		   FROM access_profiles
		  WHERE current_node_id = ?
		    AND fixed_node_id != ?
		    AND state != 'invalid_config'
		    AND type IN ('fastest', 'chain')`,
		nodeID,
		nodeID,
	)
	if err != nil {
		return err
	}
	type profileRef struct {
		id          string
		name        string
		profileType string
		version     int64
		autoEval    bool
	}
	var profiles []profileRef
	for rows.Next() {
		var profile profileRef
		var autoEval int
		if err := rows.Scan(&profile.id, &profile.name, &profile.profileType, &profile.version, &autoEval); err != nil {
			_ = rows.Close()
			return err
		}
		profile.autoEval = autoEval == 1
		profiles = append(profiles, profile)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.autoEval {
			if _, err := tx.Exec(
				`UPDATE access_profiles
				    SET current_node_id = '',
				        current_exit_node_id = CASE WHEN current_exit_node_id = ? THEN '' ELSE current_exit_node_id END,
				        state = 'waiting_observation',
				        last_error = '',
				        switch_reason = 'current_node_removed',
				        last_evaluation_started_at = ?
				  WHERE id = ?`,
				nodeID,
				unixMillisNow(),
				profile.id,
			); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(
			`UPDATE access_profiles
			    SET current_node_id = '',
			        current_exit_node_id = CASE WHEN current_exit_node_id = ? THEN '' ELSE current_exit_node_id END,
			        state = 'pending',
			        last_error = 'current node no longer exists',
			        switch_reason = 'current_node_removed'
			  WHERE id = ?`,
			nodeID,
			profile.id,
		); err != nil {
			return err
		}
	}
	return nil
}

func (g *Gateway) enqueueProfileEvaluationsWaitingForObservation() {
	rows, err := g.db.Query(
		`SELECT id, name, config_version
		   FROM access_profiles
		  WHERE state = 'waiting_observation'
		    AND auto_evaluation_enabled = 1
		    AND type IN ('fastest', 'chain')
		  ORDER BY created_at, id`,
	)
	if err != nil {
		return
	}
	type profileRef struct {
		id      string
		name    string
		version int64
	}
	var profiles []profileRef
	for rows.Next() {
		var profile profileRef
		if err := rows.Scan(&profile.id, &profile.name, &profile.version); err == nil {
			profiles = append(profiles, profile)
		}
	}
	if err := rows.Close(); err != nil {
		return
	}
	for _, profile := range profiles {
		if g.hasUnfinishedCurrentNodeObservedEvaluation(profile.id) {
			continue
		}
		_, _ = g.enqueueProfileEvaluationRun(profile.id, profile.name, "current_node_observed", profile.version, true)
	}
}

func (g *Gateway) hasUnfinishedCurrentNodeObservedEvaluation(profileID string) bool {
	var exists int
	err := g.db.QueryRow(
		`SELECT 1
		   FROM maintenance_runs
		  WHERE run_type = ?
		    AND target_id = ?
		    AND trigger_source = 'current_node_observed'
		    AND state IN (?, ?)
		  LIMIT 1`,
		maintenanceTaskProfileEvaluation,
		profileID,
		maintenanceRunStateQueued,
		maintenanceRunStateRunning,
	).Scan(&exists)
	return err == nil
}

func invalidateProfilesForDeletedSubscriptionTx(tx *sql.Tx, subscriptionID string) error {
	rows, err := tx.Query(`SELECT id, node_source_mode, source_ids_json, manual_only FROM access_profiles`)
	if err != nil {
		return err
	}
	type profileRef struct {
		id string
	}
	var invalid []profileRef
	for rows.Next() {
		var id, nodeSourceMode, sourceIDsJSON string
		var manualOnly int
		if err := rows.Scan(&id, &nodeSourceMode, &sourceIDsJSON, &manualOnly); err != nil {
			_ = rows.Close()
			return err
		}
		var sourceIDs []string
		_ = json.Unmarshal([]byte(sourceIDsJSON), &sourceIDs)
		if normalizeNodeSourceMode(nodeSourceMode, sourceIDs, manualOnly == 1) != "specific_subscriptions" {
			continue
		}
		for _, sourceID := range sourceIDs {
			if sourceID == subscriptionID {
				invalid = append(invalid, profileRef{id: id})
				break
			}
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, profile := range invalid {
		if _, err := tx.Exec(`UPDATE access_profiles SET current_node_id = '', state = 'invalid_config' WHERE id = ?`, profile.id); err != nil {
			return err
		}
	}
	return nil
}

func (g *Gateway) storeSubscriptionRefreshError(subscriptionID, errorText string) {
	_, _ = g.db.Exec(
		`UPDATE subscriptions SET last_error = ?, updated_at = ? WHERE id = ?`,
		errorText,
		unixMillisNow(),
		subscriptionID,
	)
}

func skippedSummaryRowsFromJSON(text string) []skippedEntrySummary {
	var rows []skippedEntrySummary
	if err := json.Unmarshal([]byte(text), &rows); err != nil {
		return nil
	}
	return rows
}

func parseSubscriptionNodes(data []byte) ([]parsedSubscriptionNode, skippedEntrySummarySet, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil, errors.New("subscription is empty")
	}
	if decoded, ok := tryBase64Text(trimmed); ok {
		nodes, skippedSummary, err := parseSubscriptionNodes([]byte(decoded))
		if err == nil {
			return nodes, skippedSummary, nil
		}
	}
	var rawOutbounds []json.RawMessage
	if strings.HasPrefix(trimmed, "{") {
		var obj struct {
			Outbounds   []json.RawMessage `json:"outbounds"`
			Proxies     []map[string]any  `json:"proxies"`
			ProxyGroups []map[string]any  `json:"proxy-groups"`
		}
		if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
			return nil, nil, err
		}
		if len(obj.Proxies) > 0 {
			nodes, skippedSummary := parseClashProxyMaps(obj.Proxies)
			addClashProxyGroupSkips(skippedSummary, obj.ProxyGroups)
			return deduplicateParsedSubscriptionNodes(nodes, skippedSummary), skippedSummary, nil
		}
		rawOutbounds = obj.Outbounds
	} else if looksLikeJSONArray(trimmed) {
		if err := json.Unmarshal([]byte(trimmed), &rawOutbounds); err != nil {
			return nil, nil, err
		}
	} else {
		if strings.Contains(trimmed, "proxies:") {
			var cfg struct {
				Proxies     []map[string]any `yaml:"proxies"`
				ProxyGroups []map[string]any `yaml:"proxy-groups"`
			}
			if err := yaml.Unmarshal([]byte(trimmed), &cfg); err != nil {
				return nil, nil, err
			}
			if len(cfg.Proxies) > 0 || len(cfg.ProxyGroups) > 0 {
				nodes, skippedSummary := parseClashProxyMaps(cfg.Proxies)
				addClashProxyGroupSkips(skippedSummary, cfg.ProxyGroups)
				return deduplicateParsedSubscriptionNodes(nodes, skippedSummary), skippedSummary, nil
			}
		}
		if nodes, skippedSummary, recognized := parseSurgeSubscription(trimmed); recognized {
			return deduplicateParsedSubscriptionNodes(nodes, skippedSummary), skippedSummary, nil
		}
		nodes, skippedSummary := parseURILines(trimmed)
		if len(nodes) > 0 || skippedSummary.count() > 0 {
			return deduplicateParsedSubscriptionNodes(nodes, skippedSummary), skippedSummary, nil
		}
		return nil, nil, errors.New("unsupported subscription format")
	}
	var nodes []parsedSubscriptionNode
	skippedSummary := skippedEntrySummarySet{}
	for _, raw := range rawOutbounds {
		node, reason, detail := parseSingBoxOutboundNodeWithDetail(raw)
		if reason != "" {
			skippedSummary.addDetail(reason, detail)
			continue
		}
		nodes = append(nodes, node)
	}
	return deduplicateParsedSubscriptionNodes(nodes, skippedSummary), skippedSummary, nil
}

func looksLikeJSONArray(text string) bool {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "[") {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(text, "["))
	return rest == "" || strings.HasPrefix(rest, "{") || strings.HasPrefix(rest, "]")
}

func addClashProxyGroupSkips(summary skippedEntrySummarySet, groups []map[string]any) {
	for _, group := range groups {
		summary.addDetail(skipReasonClashProxyGroupIgnored, skippedEntryDetail{
			Name:      strings.TrimSpace(anyString(group["name"])),
			EntryType: strings.TrimSpace(anyString(group["type"])),
		})
	}
}

func deduplicateParsedSubscriptionNodes(nodes []parsedSubscriptionNode, skippedSummary skippedEntrySummarySet) []parsedSubscriptionNode {
	if len(nodes) < 2 {
		return nodes
	}
	seen := map[string]struct{}{}
	deduped := make([]parsedSubscriptionNode, 0, len(nodes))
	for _, node := range nodes {
		outboundJSON, err := normalizedNodeOutboundJSON(node)
		if err != nil {
			skippedSummary.addDetail(skipReasonUnsupportedOption, skippedEntryDetail{
				Name:      node.Name,
				EntryType: node.Type,
				Detail:    err.Error(),
			})
			continue
		}
		fingerprint := outboundFingerprint(outboundJSON)
		if _, ok := seen[fingerprint]; ok {
			skippedSummary.addDetail(skipReasonDuplicateNode, skippedEntryDetail{
				Name:      node.Name,
				EntryType: node.Type,
			})
			continue
		}
		seen[fingerprint] = struct{}{}
		deduped = append(deduped, node)
	}
	return deduped
}
