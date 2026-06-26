package app

import (
	"bufio"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	appobservations "proxygateway/internal/application/observations"
)

const defaultEgressIPProbeURL = "https://cloudflare.com/cdn-cgi/trace"

func (g *Gateway) handleRunNodeObservations(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		TestURL  string   `json:"test_url"`
		ProbeURL string   `json:"probe_url"`
		NodeID   string   `json:"node_id"`
		NodeIDs  []string `json:"node_ids"`
	}
	if err := readJSON(r, &req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	plan, err := appobservations.PlanManualRun(manualObservationRunRepository{g: g}, appobservations.ManualRunCommand{
		NodeID:          req.NodeID,
		NodeIDs:         req.NodeIDs,
		ProbeURL:        req.ProbeURL,
		LegacyTestURL:   req.TestURL,
		DefaultProbeURL: defaultEgressIPProbeURL,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, appobservations.ErrObservationTargetNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	if plan.CancelUnfinishedAggregateRuns {
		if err := g.cancelUnfinishedNodeObservationAggregateRuns("replaced_by_manual_run"); err != nil {
			writeError(w, http.StatusInternalServerError, "cancel previous node observation runs")
			return
		}
	}
	run, err := g.createNodeObservationRun("manual", plan.Scope, toObservationNodeRecords(plan.Targets), plan.ProbeURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create maintenance run")
		return
	}
	if err := g.runNodeObservationMaintenanceRun(run.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "run node observation")
		return
	}
	finished, _ := g.loadMaintenanceRun(run.ID)
	successCount, _ := finished.detail()["success_count"].(float64)
	writeJSON(w, http.StatusOK, map[string]any{"observed_nodes": int(successCount), "run_id": run.ID})
}

func (g *Gateway) runNodeObservationMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		return err
	}
	if run.State == maintenanceRunStateFinished {
		return nil
	}
	detail := run.detail()
	probeURL, _ := detail["probe_url"].(string)
	probeURL = effectiveEgressIPProbeURL(probeURL, "")
	targets := g.nodeObservationRunTargets(run, detail)
	if len(targets) == 0 {
		outcome := appobservations.BuildNoTargetOutcome()
		detail["success_count"] = outcome.SuccessCount
		detail["failure_count"] = outcome.FailureCount
		detail["failure_reasons"] = outcome.FailureReasons
		detail["sample_failures"] = outcome.SampleFailures
		return g.finishMaintenanceRun(run.ID, outcome.Result, outcome.ReasonCode, outcome.FinishedCount, detail, outcome.LastError)
	}
	settings, err := g.loadMaintenanceSettings()
	if err != nil {
		return err
	}
	evalSettings, err := g.loadEvaluationSettings()
	if err != nil {
		return err
	}
	concurrency := settings.NodeObservationConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(targets) {
		concurrency = len(targets)
	}
	if err := g.startMaintenanceRun(run.ID); err != nil {
		return err
	}
	jobs := make(chan nodeRecord)
	results := make(chan appobservations.RunResult, len(targets))
	var wg sync.WaitGroup
	persistence := nodeObservationPersistenceRepository{db: g.db}
	lookup := geoIPCountryLookup{geoIP: g.geoIP}
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range jobs {
				results <- appobservations.ExecuteNodeObservation(
					persistence,
					lookup,
					nodeObservationProbeExecutor{g: g, node: node, probeURL: probeURL, settings: evalSettings},
					appobservations.NodeTarget{ID: node.ID, Name: node.Name},
					unixMillisNow(),
				)
			}
		}()
	}
	for _, node := range targets {
		jobs <- node
	}
	close(jobs)
	wg.Wait()
	close(results)
	runResults := make([]appobservations.RunResult, 0, len(targets))
	for result := range results {
		runResults = append(runResults, result)
	}
	outcome := appobservations.BuildCompletedOutcome(run.TriggerSource, runResults)
	detail["success_count"] = outcome.SuccessCount
	detail["failure_count"] = outcome.FailureCount
	detail["failure_reasons"] = outcome.FailureReasons
	detail["sample_failures"] = outcome.SampleFailures
	if err := g.finishMaintenanceRun(run.ID, outcome.Result, outcome.ReasonCode, outcome.FinishedCount, detail, outcome.LastError); err != nil {
		return err
	}
	if outcome.EnqueueWaitingProfiles {
		g.enqueueProfileEvaluationsWaitingForObservation()
	}
	return nil
}

type manualObservationRunRepository struct {
	g *Gateway
}

func (r manualObservationRunRepository) EnabledNodeByID(nodeID string) (appobservations.NodeTarget, bool, error) {
	node, err := r.g.loadNode(nodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appobservations.NodeTarget{}, false, nil
		}
		return appobservations.NodeTarget{}, false, err
	}
	if !node.Enabled {
		return appobservations.NodeTarget{}, false, nil
	}
	return appobservations.NodeTarget{ID: node.ID, Name: node.Name}, true, nil
}

func (r manualObservationRunRepository) AllEnabledNodes() ([]appobservations.NodeTarget, error) {
	rows, err := r.g.db.Query(`SELECT id, name FROM nodes WHERE enabled = 1 ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []appobservations.NodeTarget
	for rows.Next() {
		var target appobservations.NodeTarget
		if err := rows.Scan(&target.ID, &target.Name); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func toObservationNodeRecords(targets []appobservations.NodeTarget) []nodeRecord {
	out := make([]nodeRecord, 0, len(targets))
	for _, target := range targets {
		out = append(out, nodeRecord{ID: target.ID, Name: target.Name, Enabled: true})
	}
	return out
}

func toObservationNodeTargets(targets []nodeRecord) []appobservations.NodeTarget {
	out := make([]appobservations.NodeTarget, 0, len(targets))
	for _, target := range targets {
		out = append(out, appobservations.NodeTarget{ID: target.ID, Name: target.Name})
	}
	return out
}

func (g *Gateway) createNodeObservationRun(triggerSource, scope string, targets []nodeRecord, probeURL string) (maintenanceRunRecord, error) {
	targetID, targetLabel := "", ""
	if scope == "single_node" && len(targets) == 1 {
		targetID = targets[0].ID
		targetLabel = targets[0].Name
	}
	detail := map[string]any{
		"target_scope": scope,
		"probe_url":    probeURL,
		"node_ids":     nodeIDsForRunTargets(targets),
	}
	return g.createMaintenanceRun(maintenanceTaskNodeObservation, triggerSource, targetID, targetLabel, len(targets), detail)
}

func nodeIDsForRunTargets(targets []nodeRecord) []string {
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ID)
	}
	return ids
}

func (g *Gateway) nodeObservationRunTargets(run maintenanceRunRecord, detail map[string]any) []nodeRecord {
	ids := stringSliceFromDetail(detail["node_ids"])
	if len(ids) == 0 && run.TargetID != "" {
		ids = []string{run.TargetID}
	}
	targets := make([]nodeRecord, 0, len(ids))
	for _, id := range ids {
		node, err := g.loadNode(id)
		if err == nil && node.Enabled {
			targets = append(targets, node)
		}
	}
	return targets
}

func stringSliceFromDetail(value any) []string {
	switch typed := value.(type) {
	case []string:
		return normalizeStringList(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return normalizeStringList(values)
	default:
		return []string{}
	}
}

func effectiveEgressIPProbeURL(probeURL, legacyTestURL string) string {
	probeURL = strings.TrimSpace(probeURL)
	if probeURL != "" {
		return probeURL
	}
	legacyTestURL = strings.TrimSpace(legacyTestURL)
	if legacyTestURL != "" {
		return legacyTestURL
	}
	return defaultEgressIPProbeURL
}

func (g *Gateway) observeNode(node nodeRecord, probeURL string, settings evaluationSettings) (bool, error) {
	result := appobservations.ExecuteNodeObservation(
		nodeObservationPersistenceRepository{db: g.db},
		geoIPCountryLookup{geoIP: g.geoIP},
		nodeObservationProbeExecutor{g: g, node: node, probeURL: probeURL, settings: settings},
		appobservations.NodeTarget{ID: node.ID, Name: node.Name},
		unixMillisNow(),
	)
	if result.OK {
		return true, nil
	}
	if strings.TrimSpace(result.Error) == "" {
		return false, nil
	}
	return false, errors.New(result.Error)
}

type nodeObservationProbeExecutor struct {
	g        *Gateway
	node     nodeRecord
	probeURL string
	settings evaluationSettings
}

func (e nodeObservationProbeExecutor) Probe() (appobservations.ProbePayload, error) {
	start := time.Now()
	outbound, err := buildOutboundGETRequest(e.probeURL)
	if err != nil {
		return appobservations.ProbePayload{}, err
	}
	timeouts := e.settings.probeDialTimeouts()
	conn, err := e.g.dialViaNode(e.node, outbound.TargetHost, timeouts)
	if err != nil {
		return appobservations.ProbePayload{}, err
	}
	defer conn.Close()
	if !timeouts.Deadline.IsZero() {
		deadlineConn := conn
		_ = deadlineConn.SetDeadline(timeouts.Deadline)
		defer func() { _ = deadlineConn.SetDeadline(time.Time{}) }()
	}
	conn, err = wrapOutboundGETConn(conn, outbound)
	if err != nil {
		return appobservations.ProbePayload{}, err
	}
	if err := outbound.Request.Write(conn); err != nil {
		return appobservations.ProbePayload{}, err
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), outbound.Request)
	if err != nil {
		return appobservations.ProbePayload{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		err := errors.New("egress probe returned " + resp.Status)
		return appobservations.ProbePayload{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return appobservations.ProbePayload{}, err
	}
	return appobservations.ProbePayload{
		Raw:       raw,
		LatencyMS: time.Since(start).Milliseconds(),
	}, nil
}

type nodeObservationPersistenceRepository struct {
	db *sql.DB
}

func (r nodeObservationPersistenceRepository) SaveSuccess(nodeID string, record appobservations.SuccessRecord, observedAt int64) error {
	_, err := r.db.Exec(
		`INSERT INTO node_observations (node_id, usable, egress_ip, egress_country, latency_ms, last_error, last_success_at)
		 VALUES (?, 1, ?, ?, ?, '', ?)
		 ON CONFLICT(node_id) DO UPDATE SET
			usable = 1,
			egress_ip = excluded.egress_ip,
			egress_country = excluded.egress_country,
			latency_ms = excluded.latency_ms,
			last_error = '',
			last_success_at = excluded.last_success_at`,
		nodeID,
		record.EgressIP,
		record.EgressCountry,
		record.LatencyMS,
		observedAt,
	)
	return err
}

func (r nodeObservationPersistenceRepository) SaveFailure(nodeID, errorText string, observedAt int64) error {
	_, err := r.db.Exec(
		`INSERT INTO node_observations (node_id, usable, last_error, last_failure_at)
		 VALUES (?, 0, ?, ?)
		 ON CONFLICT(node_id) DO UPDATE SET
			usable = 0,
			last_error = excluded.last_error,
			last_failure_at = excluded.last_failure_at`,
		nodeID,
		errorText,
		observedAt,
	)
	return err
}

type geoIPCountryLookup struct {
	geoIP geoIPCountryService
}

func (l geoIPCountryLookup) LookupCountry(ip string) string {
	if l.geoIP == nil {
		return ""
	}
	return l.geoIP.LookupCountry(ip)
}

func (g *Gateway) nodeObservation(nodeID string) map[string]any {
	var usable int
	var egressIP, egressCountry, lastError string
	var latencyMS, lastSuccessAt, lastFailureAt int64
	err := g.db.QueryRow(
		`SELECT usable, egress_ip, egress_country, latency_ms, last_error, last_success_at, last_failure_at
		 FROM node_observations WHERE node_id = ?`,
		nodeID,
	).Scan(&usable, &egressIP, &egressCountry, &latencyMS, &lastError, &lastSuccessAt, &lastFailureAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return map[string]any{"usable": false}
	}
	return map[string]any{
		"usable":          usable == 1,
		"egress_ip":       egressIP,
		"egress_country":  egressCountry,
		"latency_ms":      latencyMS,
		"last_error":      lastError,
		"last_success_at": lastSuccessAt,
		"last_failure_at": lastFailureAt,
		"stale":           usable == 0 && lastSuccessAt > 0 && lastFailureAt >= lastSuccessAt,
	}
}
