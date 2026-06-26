package app

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	appproxy "proxygateway/internal/application/proxy"

	"go.uber.org/zap"
)

const (
	requestFailureStageAuthentication   = appproxy.FailureStageAuthentication
	requestFailureStageProfileSelection = appproxy.FailureStageProfileSelection
	requestFailureStagePathSelection    = appproxy.FailureStagePathSelection
	requestFailureStageDial             = appproxy.FailureStageDial
	requestFailureStageProxyHandshake   = appproxy.FailureStageProxyHandshake
	requestFailureStageUpstream         = appproxy.FailureStageUpstream

	requestLogQueueCapacity = 4096
	requestLogFlushTimeout  = 2 * time.Second

	requestLogDropWarnEvery = 100
)

type requestLogEventKind int

const (
	requestLogEventStart requestLogEventKind = iota + 1
	requestLogEventFinish
	requestLogEventFailure
)

type requestLogEvent struct {
	kind                    requestLogEventKind
	id                      string
	ts                      int64
	proxyCredentialID       string
	proxyCredential         string
	accessProfileID         string
	accessProfile           string
	accessProfileIdentifier string
	targetHost              string
	proxyPath               string
	proxyPathJSON           string
	success                 bool
	failureStage            string
	errorText               string
	httpStatus              int
	durationMS              int64
	ingressBytes            int64
	egressBytes             int64
}

type requestLogWriter struct {
	db      *sql.DB
	events  chan requestLogEvent
	stop    chan struct{}
	done    chan struct{}
	mu      sync.Mutex
	closing bool
	dropped atomic.Int64
	logger  *zap.Logger
}

func newRequestLogWriter(db *sql.DB, logger *zap.Logger) *requestLogWriter {
	w := &requestLogWriter{
		db:     db,
		events: make(chan requestLogEvent, requestLogQueueCapacity),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		logger: ensureLogger(logger),
	}
	go w.run()
	return w
}

func (w *requestLogWriter) enqueue(event requestLogEvent) bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing {
		w.recordDrop(event, "closing")
		return false
	}
	select {
	case w.events <- event:
		return true
	default:
		w.recordDrop(event, "queue_full")
		return false
	}
}

func (w *requestLogWriter) recordDrop(event requestLogEvent, reason string) {
	dropped := w.dropped.Add(1)
	if dropped == 1 || dropped%requestLogDropWarnEvery == 0 {
		ensureLogger(w.logger).Warn("request log event dropped",
			zap.String("reason", reason),
			zap.Int64("dropped_total", dropped),
			zap.String("event_kind", requestLogEventKindName(event.kind)),
			zap.String("log_id", event.id),
			zap.String("target_host", event.targetHost),
			zap.String("failure_stage", event.failureStage),
		)
	}
}

func (w *requestLogWriter) droppedCount() int64 {
	if w == nil {
		return 0
	}
	return w.dropped.Load()
}

func (w *requestLogWriter) close(timeout time.Duration) bool {
	if w == nil {
		return true
	}
	w.mu.Lock()
	if !w.closing {
		w.closing = true
		close(w.stop)
	}
	w.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-w.done:
		return true
	case <-timer.C:
		return false
	}
}

func (w *requestLogWriter) run() {
	defer close(w.done)
	for {
		select {
		case event := <-w.events:
			w.write(event)
		case <-w.stop:
			for {
				select {
				case event := <-w.events:
					w.write(event)
				default:
					return
				}
			}
		}
	}
}

func (w *requestLogWriter) write(event requestLogEvent) {
	var err error
	switch event.kind {
	case requestLogEventStart:
		_, err = w.db.Exec(
			`INSERT INTO request_logs (
				id, ts, proxy_credential_id, proxy_credential, access_profile_id, access_profile, access_profile_identifier,
				target_host, proxy_path, proxy_path_json, state, success, failure_stage, error, duration_ms, ingress_bytes, egress_bytes, http_status
			 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'running', NULL, '', '', 0, 0, 0, 0)`,
			event.id,
			event.ts,
			event.proxyCredentialID,
			event.proxyCredential,
			event.accessProfileID,
			event.accessProfile,
			event.accessProfileIdentifier,
			event.targetHost,
			event.proxyPath,
			event.proxyPathJSON,
		)
	case requestLogEventFinish:
		failureStage := event.failureStage
		if event.success {
			failureStage = ""
		}
		successInt := 0
		if event.success {
			successInt = 1
		}
		_, err = w.db.Exec(
			`UPDATE request_logs
			    SET state = 'completed',
			        success = ?,
			        failure_stage = ?,
			        error = ?,
			        duration_ms = ?,
			        ingress_bytes = ?,
			        egress_bytes = ?,
			        http_status = ?
			  WHERE id = ?`,
			successInt,
			failureStage,
			event.errorText,
			event.durationMS,
			event.ingressBytes,
			event.egressBytes,
			event.httpStatus,
			event.id,
		)
	case requestLogEventFailure:
		_, err = w.db.Exec(
			`INSERT INTO request_logs (
				id, ts, proxy_credential_id, proxy_credential, access_profile_id, access_profile, access_profile_identifier,
				target_host, proxy_path, proxy_path_json, state, success, failure_stage, error, duration_ms, ingress_bytes, egress_bytes, http_status
			 ) VALUES (?, ?, '', '', '', ?, ?, ?, '', '', 'completed', 0, ?, ?, ?, 0, 0, ?)`,
			event.id,
			event.ts,
			event.accessProfile,
			event.accessProfileIdentifier,
			event.targetHost,
			event.failureStage,
			event.errorText,
			event.durationMS,
			event.httpStatus,
		)
	default:
		ensureLogger(w.logger).Warn("unknown request log event kind",
			zap.Int("event_kind", int(event.kind)),
			zap.String("log_id", event.id),
		)
		return
	}
	if err != nil {
		ensureLogger(w.logger).Error("write request log event failed",
			zap.String("event_kind", requestLogEventKindName(event.kind)),
			zap.String("log_id", event.id),
			zap.String("target_host", event.targetHost),
			zap.String("failure_stage", event.failureStage),
			zap.Error(err),
		)
	}
}

func requestLogEventKindName(kind requestLogEventKind) string {
	switch kind {
	case requestLogEventStart:
		return "start"
	case requestLogEventFinish:
		return "finish"
	case requestLogEventFailure:
		return "failure"
	default:
		return "unknown"
	}
}

func (g *Gateway) handleRequestLogs(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	page, pageSize := parsePagination(r)
	offset := (page - 1) * pageSize
	where, args := requestLogFilters(r)

	var total int
	countQuery := `SELECT COUNT(*) FROM request_logs` + where
	if err := g.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "count request logs")
		return
	}

	query := `SELECT id, ts, proxy_credential_id, proxy_credential, access_profile_id, access_profile, access_profile_identifier,
	                target_host, proxy_path, proxy_path_json, state, success, failure_stage, error, duration_ms, ingress_bytes, egress_bytes, http_status
		 FROM request_logs` + where + ` ORDER BY ts DESC, id DESC LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := g.db.Query(query, queryArgs...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list request logs")
		return
	}
	defer rows.Close()
	var logs []map[string]any
	for rows.Next() {
		var id, proxyCredentialID, proxyCredential, accessProfileID, accessProfile, profileIdentifier, targetHost, proxyPath, proxyPathJSON, state, failureStage, errorText string
		var ts, durationMS, ingressBytes, egressBytes int64
		var success sql.NullInt64
		var httpStatus int
		if err := rows.Scan(&id, &ts, &proxyCredentialID, &proxyCredential, &accessProfileID, &accessProfile, &profileIdentifier, &targetHost, &proxyPath, &proxyPathJSON, &state, &success, &failureStage, &errorText, &durationMS, &ingressBytes, &egressBytes, &httpStatus); err != nil {
			writeError(w, http.StatusInternalServerError, "scan request log")
			return
		}
		result, successValue := requestLogResult(state, success)
		if state == "running" && durationMS <= 0 {
			durationMS = unixMillisNow() - ts
			if durationMS <= 0 {
				durationMS = 1
			}
		}
		var httpStatusPtr any = nil
		if httpStatus > 0 {
			httpStatusPtr = httpStatus
		}
		var credentialID any = proxyCredentialID
		if proxyCredentialID == "" {
			credentialID = nil
		}
		logs = append(logs, map[string]any{
			"id":               id,
			"occurred_at":      ts,
			"access_profile":   map[string]any{"id": accessProfileID, "name": accessProfile, "profile_identifier": profileIdentifier},
			"proxy_credential": map[string]any{"id": credentialID, "remark": proxyCredential},
			"target_host":      targetHost,
			"target_port":      targetPortFromTarget(targetHost),
			"target":           targetHost,
			"proxy_path":       parseRequestLogProxyPath(proxyPathJSON),
			"proxy_path_label": proxyPath,
			"state":            state,
			"result":           result,
			"success":          successValue,
			"failure_stage":    failureStage,
			"error":            errorText,
			"duration_ms":      durationMS,
			"ingress_bytes":    ingressBytes,
			"egress_bytes":     egressBytes,
			"http_status":      httpStatusPtr,
		})
	}
	if logs == nil {
		logs = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": logs, "request_logs": logs, "total": total, "page": page, "page_size": pageSize})
}

func requestLogFilters(r *http.Request) (string, []any) {
	query := r.URL.Query()
	var clauses []string
	var args []any
	if value := strings.TrimSpace(firstNonEmpty(query.Get("access_profile_id"), query.Get("access_profile"))); value != "" {
		clauses = append(clauses, "(access_profile_id = ? OR access_profile = ?)")
		args = append(args, value, value)
	}
	if value := strings.TrimSpace(firstNonEmpty(query.Get("credential_id"), query.Get("proxy_credential_id"), query.Get("proxy_credential"))); value != "" {
		clauses = append(clauses, "(proxy_credential_id = ? OR proxy_credential = ?)")
		args = append(args, value, value)
	}
	if value := strings.TrimSpace(query.Get("node_id")); value != "" {
		clauses = append(clauses, "proxy_path_json LIKE ?")
		args = append(args, "%"+value+"%")
	}
	if value := strings.TrimSpace(query.Get("target")); value != "" {
		clauses = append(clauses, "target_host LIKE ?")
		args = append(args, "%"+value+"%")
	}
	switch strings.ToLower(strings.TrimSpace(query.Get("state"))) {
	case "running":
		clauses = append(clauses, "state = 'running'")
	case "completed":
		clauses = append(clauses, "state = 'completed'")
	}
	successFilter := strings.ToLower(strings.TrimSpace(query.Get("success")))
	if successFilter == "" {
		successFilter = strings.ToLower(strings.TrimSpace(query.Get("result")))
	}
	switch successFilter {
	case "true", "1", "success", "succeeded":
		clauses = append(clauses, "state = 'completed' AND success = 1")
	case "false", "0", "failure", "failed":
		clauses = append(clauses, "state = 'completed' AND success = 0")
	case "running":
		clauses = append(clauses, "state = 'running'")
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func requestLogResult(state string, success sql.NullInt64) (string, any) {
	if state == "running" {
		return "running", nil
	}
	if success.Valid && success.Int64 == 1 {
		return "success", true
	}
	return "failure", false
}

func (g *Gateway) startProxyRequest(path selectedProxyPath, targetHost string, startedAt time.Time) string {
	id, err := prefixedID("log")
	if err != nil {
		return ""
	}
	record := appproxy.BuildRequestLogStart(appproxy.RequestLogStartInput{
		ID:         id,
		Timestamp:  startedAt.UnixMilli(),
		TargetHost: targetHost,
		Credential: appproxy.ProxyCredentialSnapshot{
			ID:     path.Credential.ID,
			Remark: path.Credential.Remark,
		},
		Profile: appproxy.AccessProfileSnapshot{
			ID:         path.ProfileID,
			Name:       path.Profile,
			Identifier: path.ProfileIdentifier,
		},
		Path: toProxyPathSnapshot(path),
	})
	g.enqueueRequestLog(requestLogEvent{
		kind:                    requestLogEventStart,
		id:                      record.ID,
		ts:                      record.Timestamp,
		proxyCredentialID:       record.ProxyCredentialID,
		proxyCredential:         record.ProxyCredential,
		accessProfileID:         record.AccessProfileID,
		accessProfile:           record.AccessProfile,
		accessProfileIdentifier: record.AccessProfileIdentifier,
		targetHost:              record.TargetHost,
		proxyPath:               record.ProxyPath,
		proxyPathJSON:           record.ProxyPathJSON,
	})
	return id
}

func (g *Gateway) finishProxyRequest(id string, success bool, failureStage string, errorText string, httpStatus int, durationMS, ingressBytes, egressBytes int64) {
	if id == "" {
		return
	}
	if success {
		g.log().Debug("proxy request completed",
			zap.String("log_id", id),
			zap.Int("http_status", httpStatus),
			zap.Int64("duration_ms", durationMS),
			zap.Int64("ingress_bytes", ingressBytes),
			zap.Int64("egress_bytes", egressBytes),
		)
	} else {
		g.log().Warn("proxy request failed",
			zap.String("log_id", id),
			zap.String("failure_stage", failureStage),
			zap.String("error", errorText),
			zap.Int("http_status", httpStatus),
			zap.Int64("duration_ms", durationMS),
		)
	}
	record := appproxy.BuildRequestLogFinish(appproxy.RequestLogFinishInput{
		ID:           id,
		Success:      success,
		FailureStage: failureStage,
		Error:        errorText,
		HTTPStatus:   httpStatus,
		DurationMS:   durationMS,
		IngressBytes: ingressBytes,
		EgressBytes:  egressBytes,
	})
	g.enqueueRequestLog(requestLogEvent{
		kind:         requestLogEventFinish,
		id:           record.ID,
		success:      record.Success,
		failureStage: record.FailureStage,
		errorText:    record.Error,
		httpStatus:   record.HTTPStatus,
		durationMS:   record.DurationMS,
		ingressBytes: record.IngressBytes,
		egressBytes:  record.EgressBytes,
	})
}

func (g *Gateway) recordProxyFailure(targetHost, profileIdentifier, failureStage, errorText string, httpStatus int, startedAt time.Time) {
	if strings.TrimSpace(targetHost) == "" {
		return
	}
	id, err := prefixedID("log")
	if err != nil {
		return
	}
	durationMS := elapsedMilliseconds(startedAt)
	g.log().Warn("proxy request rejected",
		zap.String("log_id", id),
		zap.String("target_host", targetHost),
		zap.String("profile_identifier", profileIdentifier),
		zap.String("failure_stage", failureStage),
		zap.String("error", errorText),
		zap.Int("http_status", httpStatus),
		zap.Int64("duration_ms", durationMS),
	)
	record := appproxy.BuildRequestLogFailure(appproxy.RequestLogFailureInput{
		ID:                id,
		Timestamp:         startedAt.UnixMilli(),
		TargetHost:        targetHost,
		ProfileIdentifier: profileIdentifier,
		FailureStage:      failureStage,
		Error:             errorText,
		HTTPStatus:        httpStatus,
		DurationMS:        durationMS,
	})
	g.enqueueRequestLog(requestLogEvent{
		kind:                    requestLogEventFailure,
		id:                      record.ID,
		ts:                      record.Timestamp,
		accessProfile:           record.AccessProfile,
		accessProfileIdentifier: record.AccessProfileIdentifier,
		targetHost:              record.TargetHost,
		failureStage:            record.FailureStage,
		errorText:               record.Error,
		durationMS:              record.DurationMS,
		httpStatus:              record.HTTPStatus,
	})
}

func (g *Gateway) enqueueRequestLog(event requestLogEvent) {
	if g == nil || g.requestLogs == nil {
		return
	}
	g.requestLogs.enqueue(event)
}

func toProxyPathSnapshot(path selectedProxyPath) appproxy.ProxyPathSnapshot {
	return appproxy.ProxyPathSnapshot{
		Node:      toProxyNodeSnapshot(path.Node),
		FrontNode: toProxyNodeSnapshot(path.FrontNode),
		ExitNode:  toProxyNodeSnapshot(path.ExitNode),
	}
}

func toProxyNodeSnapshot(node nodeRecord) appproxy.NodeSnapshot {
	return appproxy.NodeSnapshot{
		ID:         node.ID,
		Name:       node.Name,
		Protocol:   node.Type,
		Server:     node.Server,
		ServerPort: node.ServerPort,
	}
}

func requestLogProxyPathJSON(path selectedProxyPath) string {
	var value map[string]any
	if path.FrontNode.ID != "" && path.ExitNode.ID != "" {
		value = map[string]any{
			"path_type":             "chain",
			"front_node":            requestLogNodePathSummary(path.FrontNode),
			"exit_node":             requestLogNodePathSummary(path.ExitNode),
			"final_egress_country":  egressCountryDisplay(""),
			"chain_evaluation_mode": "end_to_end",
			"latency_ms":            nil,
			"latency_kind":          nil,
			"evaluated_at":          nil,
		}
	} else if path.Node.ID != "" {
		value = map[string]any{
			"path_type":    "single",
			"node":         requestLogNodePathSummary(path.Node),
			"latency_ms":   nil,
			"latency_kind": nil,
			"evaluated_at": nil,
		}
	} else {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func requestLogNodePathSummary(node nodeRecord) map[string]any {
	return map[string]any{
		"id":                     node.ID,
		"name":                   node.Name,
		"protocol":               node.Type,
		"server":                 node.Server,
		"server_port":            node.ServerPort,
		"egress_ip":              nil,
		"egress_country":         egressCountryDisplay(""),
		"observation_latency_ms": nil,
		"last_observed_at":       nil,
	}
}

func parseRequestLogProxyPath(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}
	return value
}

func targetPortFromTarget(target string) int {
	if strings.HasSuffix(target, ":443") {
		return 443
	}
	if strings.HasSuffix(target, ":80") {
		return 80
	}
	return 0
}

func proxyPathLabel(path selectedProxyPath) string {
	if path.FrontNode.ID != "" && path.ExitNode.ID != "" {
		return nodeLabel(path.FrontNode) + " -> " + nodeLabel(path.ExitNode)
	}
	return nodeLabel(path.Node)
}

func nodeLabel(node nodeRecord) string {
	if node.Name != "" {
		return node.Name
	}
	return node.ID
}

func elapsedMilliseconds(start time.Time) int64 {
	elapsed := time.Since(start).Milliseconds()
	if elapsed <= 0 {
		return 1
	}
	return elapsed
}
