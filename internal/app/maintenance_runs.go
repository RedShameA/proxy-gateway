package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	appevaluations "proxygateway/internal/application/evaluations"
	maintenanceapp "proxygateway/internal/application/maintenance"
	appsubscriptions "proxygateway/internal/application/subscriptions"

	"go.uber.org/zap"
)

const (
	maintenanceRunTypeProfileSwitch = "profile_switch"
	maintenanceRunTypeLogCleanup    = "log_cleanup"
	maintenanceRunTypeStartup       = "startup_cleanup"

	maintenanceRunStateQueued   = "queued"
	maintenanceRunStateRunning  = "running"
	maintenanceRunStateFinished = "finished"

	maintenanceRunResultSuccess   = "success"
	maintenanceRunResultWarning   = "warning"
	maintenanceRunResultFailure   = "failure"
	maintenanceRunResultSkipped   = "skipped"
	maintenanceRunResultCancelled = "cancelled"

	maintenanceRunReasonCompleted = "completed"
)

type maintenanceRunRecord struct {
	ID            string
	RunType       string
	TriggerSource string
	TargetID      string
	TargetLabel   string
	State         string
	Result        string
	ReasonCode    string
	TotalCount    int
	FinishedCount int
	DetailJSON    string
	LastError     string
	CreatedAt     int64
	StartedAt     int64
	FinishedAt    int64
	UpdatedAt     int64
}

type maintenanceRunScanner interface {
	Scan(dest ...any) error
}

func (g *Gateway) handleMaintenanceRuns(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	if r.URL.Path != "/api/maintenance/runs" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	page, pageSize := parsePagination(r)
	filter := maintenanceRunFilters(r)
	filter.Page = page
	filter.PageSize = pageSize
	list, err := g.maintenanceRunService().List(context.Background(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list maintenance runs")
		return
	}

	items := []map[string]any{}
	for _, item := range list.Items {
		run, err := recordFromApplicationRun(item)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "map maintenance run")
			return
		}
		items = append(items, run.toMap())
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"total":     list.Total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (g *Gateway) handleMaintenanceRunDetail(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/maintenance/runs/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	run, err := g.loadMaintenanceRun(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "maintenance run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "load maintenance run")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run.toMap()})
}

func maintenanceRunFilters(r *http.Request) maintenanceapp.ListFilter {
	query := r.URL.Query()
	return maintenanceapp.ListFilter{
		RunType:  strings.TrimSpace(query.Get("run_type")),
		TargetID: strings.TrimSpace(query.Get("target_id")),
		State:    strings.TrimSpace(query.Get("state")),
		Result:   strings.TrimSpace(query.Get("result")),
	}
}

func (g *Gateway) createMaintenanceRun(runType, triggerSource, targetID, targetLabel string, totalCount int, detail map[string]any) (maintenanceRunRecord, error) {
	run, err := g.maintenanceRunService().Create(context.Background(), maintenanceapp.CreateCommand{
		RunType:       runType,
		TriggerSource: triggerSource,
		TargetID:      targetID,
		TargetLabel:   targetLabel,
		TotalCount:    totalCount,
		Detail:        detail,
	})
	if err != nil {
		g.log().Warn("create maintenance run failed",
			zap.String("run_type", runType),
			zap.String("trigger_source", triggerSource),
			zap.String("target_id", targetID),
			zap.Error(err),
		)
		return maintenanceRunRecord{}, err
	}
	record, err := recordFromApplicationRun(run)
	if err != nil {
		g.log().Warn("map maintenance run failed", zap.String("run_type", runType), zap.Error(err))
		return maintenanceRunRecord{}, err
	}
	g.log().Debug("maintenance run created",
		zap.String("run_id", record.ID),
		zap.String("run_type", record.RunType),
		zap.String("trigger_source", record.TriggerSource),
		zap.String("target_id", record.TargetID),
		zap.Int("total_count", record.TotalCount),
	)
	return record, nil
}

func (g *Gateway) enqueueProfileEvaluationRun(profileID, targetLabel, triggerSource string, configVersion int64, forceSwitch bool) (string, error) {
	detail := map[string]any{}
	if configVersion > 0 {
		detail["config_version"] = configVersion
	}
	if forceSwitch {
		detail["force_switch"] = true
	}
	run, err := g.createMaintenanceRun(maintenanceTaskProfileEvaluation, triggerSource, profileID, targetLabel, 0, detail)
	if err == nil {
		g.notifyMaintenanceRunner()
	}
	return run.ID, err
}

func enqueueProfileEvaluationRunTx(tx *sql.Tx, profileID, targetLabel, triggerSource string, configVersion int64, forceSwitch bool) (string, error) {
	id, err := prefixedID("run")
	if err != nil {
		return "", err
	}
	detail := map[string]any{}
	if configVersion > 0 {
		detail["config_version"] = configVersion
	}
	if forceSwitch {
		detail["force_switch"] = true
	}
	detailJSON, err := marshalMaintenanceRunDetail(detail)
	if err != nil {
		return "", err
	}
	now := unixMillisNow()
	_, err = tx.Exec(
		`INSERT INTO maintenance_runs (
			id, run_type, trigger_source, target_id, target_label, state, total_count,
			finished_count, detail_json, created_at, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?)`,
		id,
		maintenanceTaskProfileEvaluation,
		triggerSource,
		profileID,
		targetLabel,
		maintenanceRunStateQueued,
		detailJSON,
		now,
		now,
	)
	return id, err
}

func (g *Gateway) enqueueSubscriptionRefreshRun(subscriptionID, targetLabel, triggerSource string) (string, error) {
	run, err := g.createMaintenanceRun(maintenanceTaskSubscriptionRefresh, triggerSource, subscriptionID, targetLabel, 1, map[string]any{
		"subscription_id": subscriptionID,
	})
	if err == nil {
		g.notifyMaintenanceRunner()
	}
	return run.ID, err
}

func (g *Gateway) startMaintenanceRun(id string) error {
	if err := g.maintenanceRunService().Start(context.Background(), id); err != nil {
		g.log().Warn("start maintenance run failed", zap.String("run_id", id), zap.Error(err))
		return err
	}
	g.log().Info("maintenance run started", zap.String("run_id", id))
	return nil
}

func (g *Gateway) finishMaintenanceRun(id, result, reasonCode string, finishedCount int, detail map[string]any, lastError string) error {
	if err := g.maintenanceRunService().Finish(context.Background(), maintenanceapp.FinishCommand{
		ID:            id,
		Result:        result,
		ReasonCode:    reasonCode,
		FinishedCount: finishedCount,
		Detail:        detail,
		LastError:     lastError,
	}); err != nil {
		g.log().Warn("finish maintenance run failed",
			zap.String("run_id", id),
			zap.String("result", result),
			zap.String("reason_code", reasonCode),
			zap.Error(err),
		)
		return err
	}
	fields := []zap.Field{
		zap.String("run_id", id),
		zap.String("result", result),
		zap.String("reason_code", reasonCode),
		zap.Int("finished_count", finishedCount),
	}
	if lastError != "" {
		fields = append(fields, zap.String("last_error", lastError))
	}
	switch result {
	case maintenanceRunResultFailure:
		g.log().Warn("maintenance run finished with failure", fields...)
	case maintenanceRunResultCancelled:
		g.log().Warn("maintenance run cancelled", fields...)
	case maintenanceRunResultSkipped:
		g.log().Debug("maintenance run skipped", fields...)
	default:
		g.log().Info("maintenance run finished", fields...)
	}
	return nil
}

func (g *Gateway) setMaintenanceRunTotal(id string, totalCount int) error {
	return g.maintenanceRunService().SetTotal(context.Background(), id, totalCount)
}

func (g *Gateway) cancelUnfinishedNodeObservationAggregateRuns(reasonCode string) error {
	rows, err := g.db.Query(
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE run_type = ? AND state != ?`,
		maintenanceTaskNodeObservation,
		maintenanceRunStateFinished,
	)
	if err != nil {
		return err
	}
	var runs []maintenanceRunRecord
	for rows.Next() {
		run, err := scanMaintenanceRun(rows)
		if err == nil {
			runs = append(runs, run)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, run := range runs {
		scope, _ := run.detail()["target_scope"].(string)
		if scope != "all_nodes" && scope != "due_nodes" {
			continue
		}
		if err := g.finishMaintenanceRun(run.ID, maintenanceRunResultCancelled, reasonCode, run.FinishedCount, run.detail(), run.LastError); err != nil {
			return err
		}
	}
	return nil
}

func (g *Gateway) hasUnfinishedNodeObservationAggregateRun() bool {
	rows, err := g.db.Query(
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE run_type = ? AND state != ?`,
		maintenanceTaskNodeObservation,
		maintenanceRunStateFinished,
	)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		run, err := scanMaintenanceRun(rows)
		if err != nil {
			continue
		}
		scope, _ := run.detail()["target_scope"].(string)
		if scope == "all_nodes" || scope == "due_nodes" {
			return true
		}
	}
	return false
}

func (g *Gateway) claimQueuedMaintenanceRuns(settings maintenanceSettings) []maintenanceRunRecord {
	type runLimit struct {
		runType string
		limit   int
	}
	limits := []runLimit{
		{runType: maintenanceTaskNodeObservation, limit: settings.NodeObservationConcurrency},
		{runType: maintenanceTaskProfileEvaluation, limit: settings.ProfileEvaluationConcurrency},
		{runType: maintenanceTaskSubscriptionRefresh, limit: settings.SubscriptionConcurrency},
		{runType: maintenanceTaskGeoIPUpdate, limit: settings.GeoIPConcurrency},
		{runType: maintenanceRunTypeLogCleanup, limit: 1},
	}
	var runs []maintenanceRunRecord
	for _, item := range limits {
		limit := item.limit
		if limit <= 0 {
			limit = 1
		}
		for i := 0; i < limit; i++ {
			run, ok := g.claimNextQueuedMaintenanceRun(item.runType)
			if !ok {
				break
			}
			runs = append(runs, run)
		}
	}
	return runs
}

func (g *Gateway) claimNextQueuedMaintenanceRunBatch(settings maintenanceSettings) []maintenanceRunRecord {
	for _, item := range []struct {
		runType string
		limit   int
	}{
		{runType: maintenanceTaskSubscriptionRefresh, limit: settings.SubscriptionConcurrency},
		{runType: maintenanceTaskNodeObservation, limit: settings.NodeObservationConcurrency},
		{runType: maintenanceTaskProfileEvaluation, limit: settings.ProfileEvaluationConcurrency},
		{runType: maintenanceTaskGeoIPUpdate, limit: settings.GeoIPConcurrency},
		{runType: maintenanceRunTypeLogCleanup, limit: 1},
	} {
		runs := g.claimQueuedMaintenanceRunsOfType(item.runType, item.limit)
		if len(runs) > 0 {
			return runs
		}
	}
	return nil
}

func (g *Gateway) claimQueuedMaintenanceRunsOfType(runType string, limit int) []maintenanceRunRecord {
	if limit <= 0 {
		limit = 1
	}
	runs := make([]maintenanceRunRecord, 0, limit)
	for i := 0; i < limit; i++ {
		run, ok := g.claimNextQueuedMaintenanceRun(runType)
		if !ok {
			break
		}
		runs = append(runs, run)
	}
	return runs
}

func (g *Gateway) claimNextQueuedMaintenanceRun(runType string) (maintenanceRunRecord, bool) {
	run, ok, err := g.maintenanceRunService().ClaimNext(context.Background(), runType)
	if err != nil {
		g.log().Warn("claim maintenance run failed", zap.String("run_type", runType), zap.Error(err))
		return maintenanceRunRecord{}, false
	}
	if !ok {
		return maintenanceRunRecord{}, false
	}
	record, err := recordFromApplicationRun(run)
	if err != nil {
		g.log().Warn("map claimed maintenance run failed", zap.String("run_type", runType), zap.Error(err))
		return maintenanceRunRecord{}, false
	}
	return record, true
}

func (g *Gateway) runMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		g.log().Warn("load maintenance run failed", zap.String("run_id", runID), zap.Error(err))
		return err
	}
	g.log().Info("maintenance run executing",
		zap.String("run_id", run.ID),
		zap.String("run_type", run.RunType),
		zap.String("trigger_source", run.TriggerSource),
		zap.String("target_id", run.TargetID),
		zap.Int("total_count", run.TotalCount),
	)
	switch run.RunType {
	case maintenanceTaskNodeObservation:
		return g.runNodeObservationMaintenanceRun(run.ID)
	case maintenanceTaskProfileEvaluation:
		return g.runProfileEvaluationMaintenanceRun(run.ID)
	case maintenanceTaskSubscriptionRefresh:
		return g.runSubscriptionRefreshMaintenanceRun(run.ID)
	case maintenanceTaskGeoIPUpdate:
		return g.runGeoIPUpdateMaintenanceRun(run.ID)
	case maintenanceRunTypeLogCleanup:
		return g.runLogCleanupMaintenanceRun(run.ID)
	default:
		err := errors.New("unknown maintenance run type")
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, "unknown_run_type", run.FinishedCount, run.detail(), err.Error())
		return err
	}
}

func (g *Gateway) cancelExpiredMaintenanceRunsOnStartup() error {
	rows, err := g.db.Query(
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE state IN (?, ?)`,
		maintenanceRunStateQueued,
		maintenanceRunStateRunning,
	)
	if err != nil {
		return err
	}
	var runs []maintenanceRunRecord
	for rows.Next() {
		run, err := scanMaintenanceRun(rows)
		if err == nil {
			runs = append(runs, run)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, run := range runs {
		if err := g.finishMaintenanceRun(run.ID, maintenanceRunResultCancelled, "expired_after_restart", run.FinishedCount, run.detail(), run.LastError); err != nil {
			return err
		}
	}
	repaired, invalid, err := g.repairDanglingProfilePathsOnStartup()
	if err != nil {
		return err
	}
	startup, err := g.createMaintenanceRun(maintenanceRunTypeStartup, "startup", "", "Startup cleanup", len(runs), map[string]any{
		"cancelled_count":        len(runs),
		"repaired_profile_count": repaired,
		"invalid_profile_count":  invalid,
	})
	if err != nil {
		return err
	}
	detail := startup.detail()
	detail["cancelled_count"] = len(runs)
	detail["repaired_profile_count"] = repaired
	detail["invalid_profile_count"] = invalid
	return g.finishMaintenanceRun(startup.ID, maintenanceRunResultSuccess, maintenanceRunReasonCompleted, len(runs), detail, "")
}

func (g *Gateway) repairDanglingProfilePathsOnStartup() (int, int, error) {
	rows, err := g.db.Query(
		`SELECT p.id, p.name, p.type, p.fixed_node_id, p.current_node_id, p.current_exit_node_id, p.auto_evaluation_enabled
		   FROM access_profiles p
		  WHERE p.state IN ('ready', 'degraded', 'running')
		    AND (
		      (p.current_node_id != '' AND NOT EXISTS (SELECT 1 FROM nodes n WHERE n.id = p.current_node_id))
		      OR (p.current_exit_node_id != '' AND NOT EXISTS (SELECT 1 FROM nodes n WHERE n.id = p.current_exit_node_id))
		      OR (p.fixed_node_id != '' AND NOT EXISTS (SELECT 1 FROM nodes n WHERE n.id = p.fixed_node_id))
		    )`,
	)
	if err != nil {
		return 0, 0, err
	}
	type danglingProfile struct {
		id          string
		name        string
		profileType string
		fixedNodeID string
		currentNode string
		currentExit string
		autoEval    bool
	}
	var profiles []danglingProfile
	for rows.Next() {
		var profile danglingProfile
		var autoEval int
		if err := rows.Scan(&profile.id, &profile.name, &profile.profileType, &profile.fixedNodeID, &profile.currentNode, &profile.currentExit, &autoEval); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		profile.autoEval = autoEval == 1
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, 0, err
	}
	repaired := 0
	invalid := 0
	now := unixMillisNow()
	for _, profile := range profiles {
		if profile.fixedNodeID != "" && nodeMissing(g.db, profile.fixedNodeID) {
			if _, err := g.db.Exec(
				`UPDATE access_profiles
				    SET current_node_id = '',
				        current_exit_node_id = '',
				        state = 'invalid_config',
				        last_error = 'referenced node no longer exists',
				        switch_reason = 'missing_fixed_node',
				        last_evaluated_at = ?
				  WHERE id = ?`,
				now,
				profile.id,
			); err != nil {
				return repaired, invalid, err
			}
			invalid++
			continue
		}
		if profile.profileType != "fastest" && profile.profileType != "chain" {
			continue
		}
		if profile.autoEval {
			if _, err := g.db.Exec(
				`UPDATE access_profiles
				    SET current_node_id = '',
				        current_exit_node_id = '',
				        state = 'waiting_observation',
				        last_error = '',
				        switch_reason = 'current_node_removed',
				        last_evaluation_started_at = ?
				  WHERE id = ?`,
				now,
				profile.id,
			); err != nil {
				return repaired, invalid, err
			}
			_, _ = g.enqueueProfileEvaluationRun(profile.id, profile.name, "current_node_observed", 0, true)
		} else {
			if _, err := g.db.Exec(
				`UPDATE access_profiles
				    SET current_node_id = '',
				        current_exit_node_id = '',
				        state = 'pending',
				        last_error = 'current node no longer exists',
				        switch_reason = 'current_node_removed'
				  WHERE id = ?`,
				profile.id,
			); err != nil {
				return repaired, invalid, err
			}
		}
		repaired++
	}
	return repaired, invalid, nil
}

func nodeMissing(db *sql.DB, nodeID string) bool {
	if nodeID == "" {
		return false
	}
	var exists int
	err := db.QueryRow(`SELECT 1 FROM nodes WHERE id = ?`, nodeID).Scan(&exists)
	return err != nil
}

func (g *Gateway) runGeoIPUpdateMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		return err
	}
	if run.State == maintenanceRunStateFinished {
		return nil
	}
	detail := run.detail()
	detail["source"] = "MetaCubeX"
	if err := g.startMaintenanceRun(run.ID); err != nil {
		return err
	}
	if g.geoIP == nil {
		err := errors.New("geoip service not available")
		detail["geoip"] = g.geoIPStatus()
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, "geoip_service_unavailable", 1, detail, err.Error())
		return err
	}
	err = g.geoIP.UpdateFromMetaCubeXLatest()
	detail["geoip"] = g.geoIPStatus()
	if err != nil {
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, "geoip_update_failed", 1, detail, err.Error())
		return err
	}
	return g.finishMaintenanceRun(run.ID, maintenanceRunResultSuccess, maintenanceRunReasonCompleted, 1, detail, "")
}

func (g *Gateway) runLogCleanupMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		return err
	}
	if run.State == maintenanceRunStateFinished {
		return nil
	}
	if err := g.startMaintenanceRun(run.ID); err != nil {
		return err
	}
	now := time.Now()
	nowMillis := now.UnixMilli()
	requestLogDeleted := 0
	logRetentionEnabled := boolKVSettingDefaultTrue(g.getKVSetting("log_retention_enabled"))
	logRetentionDays := systemSettingPositiveInt(g.getKVSetting("log_retention_days"), defaultRequestLogRetentionDays)
	if logRetentionEnabled {
		res, err := g.db.Exec(`DELETE FROM request_logs WHERE ts < ?`, nowMillis-secondsToMillis(int64(logRetentionDays*86400)))
		if err != nil {
			_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, "request_log_cleanup_failed", 0, run.detail(), err.Error())
			return err
		}
		requestLogDeleted = rowsAffectedInt(res)
	}
	maintenanceHistoryRetentionEnabled := boolKVSettingDefaultTrue(g.getKVSetting("maintenance_history_retention_enabled"))
	maintenanceHistoryRetentionDays := systemSettingPositiveInt(g.getKVSetting("maintenance_history_retention_days"), defaultMaintenanceHistoryRetentionDays)
	maintenanceDeleted := 0
	if maintenanceHistoryRetentionEnabled {
		res, err := g.db.Exec(`DELETE FROM maintenance_runs WHERE created_at < ? AND id != ?`, nowMillis-secondsToMillis(int64(maintenanceHistoryRetentionDays*86400)), run.ID)
		if err != nil {
			_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, "maintenance_history_cleanup_failed", 0, run.detail(), err.Error())
			return err
		}
		maintenanceDeleted = rowsAffectedInt(res)
	}
	detail := run.detail()
	detail["deleted_request_logs"] = requestLogDeleted
	detail["deleted_maintenance_runs"] = maintenanceDeleted
	detail["log_retention_enabled"] = logRetentionEnabled
	detail["log_retention_days"] = logRetentionDays
	detail["maintenance_history_retention_enabled"] = maintenanceHistoryRetentionEnabled
	detail["maintenance_history_retention_days"] = maintenanceHistoryRetentionDays
	return g.finishMaintenanceRun(run.ID, maintenanceRunResultSuccess, maintenanceRunReasonCompleted, 0, detail, "")
}

func (g *Gateway) runSubscriptionRefreshMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		return err
	}
	if run.State == maintenanceRunStateFinished {
		return nil
	}
	detail := run.detail()
	sub, err := g.loadSubscription(run.TargetID)
	if err != nil {
		detail["subscription_id"] = run.TargetID
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, "subscription_not_found", 1, detail, err.Error())
		return err
	}
	if err := g.startMaintenanceRun(run.ID); err != nil {
		return err
	}
	content, err := g.subscriptionContentForImport(sub)
	if err != nil {
		outcome := appsubscriptions.BuildRefreshFetchFailure(sub.ID)
		if outcome.PersistLastError {
			g.storeSubscriptionRefreshError(sub.ID, err.Error())
		}
		detail["subscription_id"] = outcome.SubscriptionID
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, outcome.ReasonCode, 1, detail, err.Error())
		return err
	}
	result, err := g.refreshSubscriptionWithContent(sub, content)
	if err != nil {
		outcome := appsubscriptions.BuildRefreshImportFailure(sub.ID, errors.Is(err, errInvalidSubscriptionContent))
		if outcome.PersistLastError {
			g.storeSubscriptionRefreshError(sub.ID, err.Error())
		}
		detail["subscription_id"] = outcome.SubscriptionID
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, outcome.ReasonCode, 1, detail, err.Error())
		return err
	}
	outcome := appsubscriptions.BuildRefreshSuccessOutcome(toSubscriptionRefreshImportResult(result))
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
	if outcome.EnqueueObservation {
		g.enqueueObservationForSubscriptionNodes(sub.ID)
	}
	g.enqueueStickyProfileEvaluationsForRemovedNodes(toStickyProfileEvaluationRefs(outcome.StickyProfilesToEvaluate))
	return g.finishMaintenanceRun(run.ID, outcome.Result, outcome.ReasonCode, 1, detail, "")
}

func toSubscriptionRefreshImportResult(result subscriptionImportResult) appsubscriptions.RefreshImportResult {
	out := appsubscriptions.RefreshImportResult{
		SubscriptionID:           result.ID,
		ImportedNodes:            result.ImportedNodes,
		SkippedEntrySummary:      make([]appsubscriptions.SkippedEntrySummary, 0, len(result.SkippedEntrySummary)),
		StickyProfilesToEvaluate: make([]appsubscriptions.StickyProfileEvaluationRef, 0, len(result.stickyProfilesToEvaluate)),
	}
	for _, row := range result.SkippedEntrySummary {
		out.SkippedEntrySummary = append(out.SkippedEntrySummary, appsubscriptions.SkippedEntrySummary{
			Reason:  row.Reason,
			Count:   row.Count,
			Message: row.Message,
		})
	}
	for _, ref := range result.stickyProfilesToEvaluate {
		out.StickyProfilesToEvaluate = append(out.StickyProfilesToEvaluate, appsubscriptions.StickyProfileEvaluationRef{
			ID:            ref.ID,
			Name:          ref.Name,
			ConfigVersion: ref.ConfigVersion,
		})
	}
	return out
}

func toStickyProfileEvaluationRefs(refs []appsubscriptions.StickyProfileEvaluationRef) []stickyProfileEvaluationRef {
	out := make([]stickyProfileEvaluationRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, stickyProfileEvaluationRef{
			ID:            ref.ID,
			Name:          ref.Name,
			ConfigVersion: ref.ConfigVersion,
		})
	}
	return out
}

func (g *Gateway) runProfileEvaluationMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		return err
	}
	if run.State == maintenanceRunStateFinished {
		return nil
	}
	detail := run.detail()
	forceSwitch, _ := detail["force_switch"].(bool)
	requestedConfigVersion := int64FromDetail(detail["config_version"])
	if run.TriggerSource != "current_node_observed" && g.profileWaitingForObservation(run.TargetID) {
		detail["profile_id"] = run.TargetID
		return g.finishMaintenanceRun(run.ID, maintenanceRunResultSkipped, "waiting_for_observation", 0, detail, "")
	}
	target, settings, skipped, err := g.profileEvaluationTarget(run.TargetID, forceSwitch)
	if err != nil {
		detail["profile_id"] = run.TargetID
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, "profile_load_failed", 0, detail, err.Error())
		return err
	}
	if skipped {
		detail["profile_id"] = run.TargetID
		return g.finishMaintenanceRun(run.ID, maintenanceRunResultSkipped, "min_interval_not_reached", 0, detail, "")
	}
	if guard := appevaluations.CheckConfigVersion(appevaluations.ConfigVersionGuardInput{
		RequestedConfigVersion: requestedConfigVersion,
		CurrentConfigVersion:   target.ConfigVersion,
	}); guard.Superseded {
		detail["profile_id"] = run.TargetID
		detail["current_config_version"] = guard.CurrentConfigVersion
		return g.finishMaintenanceRun(run.ID, maintenanceRunResultCancelled, guard.ReasonCode, 0, detail, "")
	}
	candidateCount := g.profileEvaluationCandidateCount(target, settings)
	if err := g.setMaintenanceRunTotal(run.ID, candidateCount); err != nil {
		return err
	}
	if err := g.startMaintenanceRun(run.ID); err != nil {
		return err
	}
	ok := g.runOneProfileEvaluation(target, settings)
	currentConfigVersion := g.profileCurrentConfigVersion(run.TargetID)
	if guard := appevaluations.CheckConfigVersion(appevaluations.ConfigVersionGuardInput{
		RequestedConfigVersion: requestedConfigVersion,
		CurrentConfigVersion:   currentConfigVersion,
	}); guard.Superseded {
		detail["profile_id"] = run.TargetID
		detail["current_config_version"] = guard.CurrentConfigVersion
		return g.finishMaintenanceRun(run.ID, maintenanceRunResultCancelled, guard.ReasonCode, 0, detail, "")
	}
	cfg, cfgErr := g.loadAccessProfileConfig(run.TargetID)
	if cfgErr != nil {
		detail["profile_id"] = run.TargetID
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, "profile_load_failed", 0, detail, cfgErr.Error())
		return cfgErr
	}
	evaluationDetail := parseJSONObject(cfg.LastEvaluationDetailsJSON)
	for key, value := range evaluationDetail {
		detail[key] = value
	}
	candidateCount = intFromDetail(detail["candidate_count"], candidateCount)
	failureCount := intFromDetail(detail["failure_count"], 0)
	if failureCount < 0 {
		failureCount = 0
	}
	successCount := candidateCount - failureCount
	if successCount < 0 {
		successCount = 0
	}
	detail["profile_id"] = run.TargetID
	detail["profile_state"] = cfg.State
	detail["success_count"] = successCount
	detail["failure_count"] = failureCount
	detail["current_path_result"] = cfg.State
	if cfg.SwitchReason != "" {
		detail["switch_decision"] = cfg.SwitchReason
	}
	result := maintenanceRunResultSuccess
	reasonCode := firstNonEmpty(cfg.SwitchReason, maintenanceRunReasonCompleted)
	lastError := ""
	if !ok {
		lastError = cfg.LastError
		if cfg.State == "degraded" {
			result = maintenanceRunResultWarning
			reasonCode = firstNonEmpty(cfg.SwitchReason, "current_path_degraded")
		} else {
			result = maintenanceRunResultFailure
			reasonCode = firstNonEmpty(cfg.SwitchReason, "evaluation_failed")
		}
	}
	return g.finishMaintenanceRun(run.ID, result, reasonCode, candidateCount, detail, lastError)
}

func (g *Gateway) profileEvaluationCandidateCount(target profileEvaluationTarget, settings evaluationSettings) int {
	nodes, err := g.candidateNodes(target.Filter)
	if err != nil {
		return 0
	}
	switch target.Type {
	case "chain":
		fronts := excludeNodes(nodes, target.ExitNodeIDs)
		if normalizeChainEvaluationMode(target.ChainEvaluationMode) == "chain_link" {
			return len(fronts) * len(target.ExitNodeIDs)
		}
		return len(fronts) * len(target.ExitNodeIDs)
	case "fastest":
		return len(limitNodes(nodes, g.effectiveCandidateLimit(target.CandidateLimit, settings.SingleCandidateLimit)))
	default:
		return 0
	}
}

func int64FromDetail(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func intFromDetail(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func systemSettingPositiveInt(value string, fallback int) int {
	if parsed := parseInt(value); parsed > 0 {
		return parsed
	}
	return fallback
}

func rowsAffectedInt(result sql.Result) int {
	affected, err := result.RowsAffected()
	if err != nil {
		return 0
	}
	return int(affected)
}

func (g *Gateway) loadMaintenanceRun(id string) (maintenanceRunRecord, error) {
	run, err := g.maintenanceRunService().Load(context.Background(), id)
	if err != nil {
		return maintenanceRunRecord{}, err
	}
	return recordFromApplicationRun(run)
}

func scanMaintenanceRun(row maintenanceRunScanner) (maintenanceRunRecord, error) {
	var run maintenanceRunRecord
	err := row.Scan(
		&run.ID,
		&run.RunType,
		&run.TriggerSource,
		&run.TargetID,
		&run.TargetLabel,
		&run.State,
		&run.Result,
		&run.ReasonCode,
		&run.TotalCount,
		&run.FinishedCount,
		&run.DetailJSON,
		&run.LastError,
		&run.CreatedAt,
		&run.StartedAt,
		&run.FinishedAt,
		&run.UpdatedAt,
	)
	return run, err
}

func marshalMaintenanceRunDetail(detail map[string]any) (string, error) {
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (run maintenanceRunRecord) detail() map[string]any {
	return parseJSONObject(run.DetailJSON)
}

func (run maintenanceRunRecord) toMap() map[string]any {
	return map[string]any{
		"id":             run.ID,
		"run_type":       run.RunType,
		"trigger_source": run.TriggerSource,
		"target_id":      run.TargetID,
		"target_label":   run.TargetLabel,
		"state":          run.State,
		"result":         run.Result,
		"reason_code":    run.ReasonCode,
		"total_count":    run.TotalCount,
		"finished_count": run.FinishedCount,
		"detail":         run.detail(),
		"last_error":     run.LastError,
		"created_at":     run.CreatedAt,
		"started_at":     run.StartedAt,
		"finished_at":    run.FinishedAt,
		"updated_at":     run.UpdatedAt,
	}
}
