package app

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	appobservations "proxygateway/internal/application/observations"
)

const (
	maintenanceTaskSubscriptionRefresh = "subscription_refresh"
	maintenanceTaskNodeObservation     = "node_observation"
	maintenanceTaskProfileEvaluation   = "profile_evaluation"
	maintenanceTaskGeoIPUpdate         = "geoip_update"
)

type maintenanceRunner struct {
	g                              *Gateway
	started                        bool
	startMu                        sync.Mutex
	workerMu                       sync.Mutex
	scheduleMu                     sync.Mutex
	lastScheduledNodeObservationAt int64
	wake                           chan struct{}
}

func newMaintenanceRunner(g *Gateway) *maintenanceRunner {
	return &maintenanceRunner{g: g, wake: make(chan struct{}, 1)}
}

func (r *maintenanceRunner) start() {
	if r == nil || r.g == nil {
		return
	}
	r.startMu.Lock()
	defer r.startMu.Unlock()
	if r.started {
		return
	}
	r.started = true
	go r.loop()
}

func (r *maintenanceRunner) loop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	r.enqueueDueScheduledTasks()
	r.runQueuedTasks()
	for {
		select {
		case <-r.g.ctx.Done():
			return
		case <-r.wake:
			r.enqueueDueScheduledTasks()
			r.runQueuedTasks()
		case <-ticker.C:
			r.enqueueDueScheduledTasks()
			r.runQueuedTasks()
		}
	}
}

func (r *maintenanceRunner) notify() {
	if r == nil {
		return
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *maintenanceRunner) enqueueDueScheduledTasks() {
	settings, err := r.g.loadMaintenanceSettings()
	if err != nil {
		return
	}
	now := time.Now()
	nowMillis := now.UnixMilli()
	if settings.NodeObservationSeconds > 0 {
		r.enqueueNodeObservationSchedule(nowMillis, settings)
	}
	if settings.ProfileEvaluationSeconds > 0 || settings.ChainEvaluationSeconds > 0 {
		r.enqueueProfileEvaluationSchedule(nowMillis, settings)
	}
	if settings.SubscriptionRefreshSeconds > 0 {
		r.enqueueSubscriptionRefreshSchedule(nowMillis, settings)
	}
	r.enqueueGeoIPUpdateSchedule(nowMillis, settings)
	r.enqueueLogCleanupSchedule(nowMillis)
}

func (r *maintenanceRunner) enqueueNodeObservationSchedule(now int64, settings maintenanceSettings) {
	if r.recentScheduledNodeObservation(now, settings.NodeObservationSeconds) {
		return
	}
	rows, err := r.g.db.Query(
		`SELECT n.id, n.name
		   FROM nodes n
		  WHERE n.enabled = 1
		    AND NOT (
		      NOT EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id)
		      AND EXISTS (SELECT 1 FROM retained_profile_nodes r WHERE r.node_id = n.id)
		    )
		  ORDER BY n.created_at, n.id`,
	)
	if err != nil {
		return
	}
	var targets []nodeRecord
	for rows.Next() {
		var nodeID, name string
		if err := rows.Scan(&nodeID, &name); err == nil {
			targets = append(targets, nodeRecord{ID: nodeID, Name: name, Enabled: true})
		}
	}
	if err := rows.Close(); err != nil {
		return
	}
	if !r.reserveScheduledNodeObservation(now, settings.NodeObservationSeconds) {
		return
	}
	plan := appobservations.PlanScheduledAggregateRun(
		toObservationNodeTargets(targets),
		settings.EgressIPProbeURL,
		r.g.hasUnfinishedNodeObservationAggregateRun(),
	)
	if !plan.CreateRun {
		return
	}
	run, err := r.g.createNodeObservationRun(plan.TriggerSource, plan.Scope, toObservationNodeRecords(plan.Targets), plan.ProbeURL)
	if err != nil {
		return
	}
	if plan.FinishImmediately {
		detail := run.detail()
		detail["success_count"] = 0
		detail["failure_count"] = 0
		_ = r.g.finishMaintenanceRun(run.ID, plan.Result, plan.ReasonCode, 0, detail, "")
		return
	}
	if plan.NotifyRunner {
		r.g.notifyMaintenanceRunner()
	}
}

func (r *maintenanceRunner) recentScheduledNodeObservation(now int64, intervalSeconds int) bool {
	if r == nil || intervalSeconds <= 0 {
		return false
	}
	r.scheduleMu.Lock()
	defer r.scheduleMu.Unlock()
	return r.recentScheduledNodeObservationLocked(now, intervalSeconds)
}

func (r *maintenanceRunner) reserveScheduledNodeObservation(now int64, intervalSeconds int) bool {
	if r == nil {
		return false
	}
	r.scheduleMu.Lock()
	defer r.scheduleMu.Unlock()
	if r.recentScheduledNodeObservationLocked(now, intervalSeconds) {
		return false
	}
	r.lastScheduledNodeObservationAt = now
	return true
}

func (r *maintenanceRunner) recentScheduledNodeObservationLocked(now int64, intervalSeconds int) bool {
	last := r.lastScheduledNodeObservationAt
	return last > 0 && now-last < secondsToMillis(int64(intervalSeconds))
}

func (r *maintenanceRunner) enqueueProfileEvaluationSchedule(now int64, settings maintenanceSettings) {
	type profileTarget struct {
		ID            string
		Name          string
		ConfigVersion int64
	}
	rows, err := r.g.db.Query(
		`SELECT id, name, type, last_evaluated_at, auto_evaluation_enabled, auto_evaluation_interval_seconds, config_version
		   FROM access_profiles
		  WHERE type IN ('fastest', 'chain')
		  ORDER BY created_at, id`,
	)
	if err != nil {
		return
	}
	var targets []profileTarget
	for rows.Next() {
		var id, name, profileType string
		var lastEvaluatedAt, configVersion int64
		var enabled, overrideInterval int
		if err := rows.Scan(&id, &name, &profileType, &lastEvaluatedAt, &enabled, &overrideInterval, &configVersion); err != nil {
			continue
		}
		if enabled == 0 {
			continue
		}
		interval := settings.ProfileEvaluationSeconds
		if profileType == "chain" {
			interval = settings.ChainEvaluationSeconds
		}
		if overrideInterval > 0 {
			interval = overrideInterval
		}
		if interval <= 0 {
			continue
		}
		if lastEvaluatedAt == 0 || now-lastEvaluatedAt >= secondsToMillis(int64(interval)) {
			targets = append(targets, profileTarget{ID: id, Name: name, ConfigVersion: configVersion})
		}
	}
	if err := rows.Close(); err != nil {
		return
	}
	for _, target := range targets {
		_, _ = r.g.enqueueProfileEvaluationRun(target.ID, target.Name, "scheduled", target.ConfigVersion, false)
	}
}

func (r *maintenanceRunner) enqueueSubscriptionRefreshSchedule(now int64, settings maintenanceSettings) {
	type subscriptionTarget struct {
		ID   string
		Name string
	}
	rows, err := r.g.db.Query(
		`SELECT id, name, updated_at, auto_refresh_enabled, auto_refresh_interval_seconds
		   FROM subscriptions
		  WHERE source_type = 'remote'
		  ORDER BY created_at, id`,
	)
	if err != nil {
		return
	}
	var targets []subscriptionTarget
	for rows.Next() {
		var id, name string
		var updatedAt int64
		var enabled, overrideInterval int
		if err := rows.Scan(&id, &name, &updatedAt, &enabled, &overrideInterval); err != nil {
			continue
		}
		if enabled == 0 {
			continue
		}
		interval := settings.SubscriptionRefreshSeconds
		if overrideInterval > 0 {
			interval = overrideInterval
		}
		if interval > 0 && (updatedAt == 0 || now-updatedAt >= secondsToMillis(int64(interval))) {
			targets = append(targets, subscriptionTarget{ID: id, Name: name})
		}
	}
	if err := rows.Close(); err != nil {
		return
	}
	for _, target := range targets {
		_, _ = r.g.enqueueSubscriptionRefreshRun(target.ID, target.Name, "scheduled")
	}
}

func (r *maintenanceRunner) enqueueGeoIPUpdateSchedule(now int64, settings maintenanceSettings) {
	if r == nil || r.g == nil || r.g.geoIP == nil {
		return
	}
	var loadedAt, updatedAt int64
	_ = r.g.db.QueryRow(`SELECT loaded_at, updated_at FROM geoip_status WHERE id = 1`).Scan(&loadedAt, &updatedAt)
	scheduledAt, ok := geoIPScheduledMillisForDay(now, settings.GeoIPUpdateTime)
	if !ok {
		return
	}
	if loadedAt > 0 && (now < scheduledAt || updatedAt >= scheduledAt) {
		return
	}
	_, _ = r.g.createMaintenanceRun(maintenanceTaskGeoIPUpdate, "scheduled", "country.mmdb", "GeoIP Database", 1, map[string]any{
		"source": "MetaCubeX",
	})
	r.g.notifyMaintenanceRunner()
}

func (r *maintenanceRunner) enqueueLogCleanupSchedule(now int64) {
	var recent int
	_ = r.g.db.QueryRow(
		`SELECT COUNT(*) FROM maintenance_runs
		  WHERE run_type = ? AND created_at > ?`,
		maintenanceRunTypeLogCleanup,
		now-secondsToMillis(86400),
	).Scan(&recent)
	if recent > 0 {
		return
	}
	_, _ = r.g.createMaintenanceRun(maintenanceRunTypeLogCleanup, "scheduled", "", "Retention cleanup", 0, map[string]any{})
	r.g.notifyMaintenanceRunner()
}

func (r *maintenanceRunner) runQueuedTasks() {
	if r == nil || r.g == nil {
		return
	}
	if !r.workerMu.TryLock() {
		return
	}
	defer r.workerMu.Unlock()
	for {
		select {
		case <-r.g.ctx.Done():
			return
		default:
		}
		settings, err := r.g.loadMaintenanceSettings()
		if err != nil {
			return
		}
		runs := r.g.claimNextQueuedMaintenanceRunBatch(settings)
		if len(runs) == 0 {
			return
		}
		var wg sync.WaitGroup
		for _, run := range runs {
			run := run
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = r.g.runMaintenanceRun(run.ID)
			}()
		}
		wg.Wait()
	}
}

func (g *Gateway) handleMaintenanceSettings(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := g.loadMaintenanceSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load maintenance settings")
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPost, http.MethodPut:
		var req maintenanceSettings
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req = normalizeMaintenanceSettings(req)
		if err := validateMaintenanceSettings(req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := g.saveMaintenanceSettings(req); err != nil {
			writeError(w, http.StatusInternalServerError, "save maintenance settings")
			return
		}
		writeJSON(w, http.StatusOK, req)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (g *Gateway) handleGeoIPStatus(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"geoip": g.geoIPStatus()})
	case http.MethodPost:
		run, err := g.createMaintenanceRun(maintenanceTaskGeoIPUpdate, "manual", "country.mmdb", "GeoIP Database", 1, map[string]any{"source": "MetaCubeX"})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create geoip update run")
			return
		}
		if err := g.runGeoIPUpdateMaintenanceRun(run.ID); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"run_id": run.ID, "geoip": g.geoIPStatus()})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (g *Gateway) loadMaintenanceSettings() (maintenanceSettings, error) {
	var settings maintenanceSettings
	err := g.db.QueryRow(
		`SELECT subscription_refresh_seconds,
		        node_observation_seconds,
		        profile_evaluation_seconds,
		        chain_evaluation_seconds,
		        geoip_update_time,
		        egress_ip_probe_url,
		        subscription_concurrency,
		        node_observation_concurrency,
		        profile_evaluation_concurrency,
		        geoip_concurrency
		   FROM maintenance_settings WHERE id = 1`,
	).Scan(
		&settings.SubscriptionRefreshSeconds,
		&settings.NodeObservationSeconds,
		&settings.ProfileEvaluationSeconds,
		&settings.ChainEvaluationSeconds,
		&settings.GeoIPUpdateTime,
		&settings.EgressIPProbeURL,
		&settings.SubscriptionConcurrency,
		&settings.NodeObservationConcurrency,
		&settings.ProfileEvaluationConcurrency,
		&settings.GeoIPConcurrency,
	)
	return normalizeMaintenanceSettings(settings), err
}

func normalizeMaintenanceSettings(settings maintenanceSettings) maintenanceSettings {
	if settings.EgressIPProbeURL == "" {
		settings.EgressIPProbeURL = defaultEgressIPProbeURL
	}
	if settings.SubscriptionConcurrency <= 0 {
		settings.SubscriptionConcurrency = 1
	}
	if settings.NodeObservationConcurrency <= 0 {
		settings.NodeObservationConcurrency = 1
	}
	if settings.ProfileEvaluationConcurrency <= 0 {
		settings.ProfileEvaluationConcurrency = 1
	}
	if settings.GeoIPConcurrency <= 0 {
		settings.GeoIPConcurrency = 1
	}
	return settings
}

func validateMaintenanceSettings(settings maintenanceSettings) error {
	if settings.SubscriptionRefreshSeconds < 0 ||
		settings.NodeObservationSeconds < 0 ||
		settings.ProfileEvaluationSeconds < 0 ||
		settings.ChainEvaluationSeconds < 0 {
		return errors.New(validationMaintenanceIntervalsNonNegative)
	}
	if strings.TrimSpace(settings.GeoIPUpdateTime) != "" {
		if _, ok := parseMaintenanceClock(settings.GeoIPUpdateTime); !ok {
			return errors.New(validationGeoIPTimeFormat)
		}
	}
	if settings.SubscriptionConcurrency <= 0 ||
		settings.NodeObservationConcurrency <= 0 ||
		settings.ProfileEvaluationConcurrency <= 0 ||
		settings.GeoIPConcurrency <= 0 {
		return errors.New(validationMaintenanceConcurrencyPositive)
	}
	return nil
}

func (g *Gateway) saveMaintenanceSettings(settings maintenanceSettings) error {
	_, err := g.db.Exec(
		`UPDATE maintenance_settings
		    SET subscription_refresh_seconds = ?,
		        node_observation_seconds = ?,
		        profile_evaluation_seconds = ?,
		        chain_evaluation_seconds = ?,
		        geoip_update_time = ?,
		        egress_ip_probe_url = ?,
		        subscription_concurrency = ?,
		        node_observation_concurrency = ?,
		        profile_evaluation_concurrency = ?,
		        geoip_concurrency = ?
		  WHERE id = 1`,
		settings.SubscriptionRefreshSeconds,
		settings.NodeObservationSeconds,
		settings.ProfileEvaluationSeconds,
		settings.ChainEvaluationSeconds,
		settings.GeoIPUpdateTime,
		settings.EgressIPProbeURL,
		settings.SubscriptionConcurrency,
		settings.NodeObservationConcurrency,
		settings.ProfileEvaluationConcurrency,
		settings.GeoIPConcurrency,
	)
	return err
}

func parseMaintenanceClock(value string) (time.Duration, bool) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute, true
}

func geoIPScheduledMillisForDay(nowMillis int64, clock string) (int64, bool) {
	offset, ok := parseMaintenanceClock(clock)
	if !ok {
		return 0, false
	}
	now := time.UnixMilli(nowMillis).Local()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return startOfDay.Add(offset).UnixMilli(), true
}

func nextGeoIPScheduledMillis(nowMillis int64, clock string) (int64, bool) {
	offset, ok := parseMaintenanceClock(clock)
	if !ok {
		return 0, false
	}
	now := time.UnixMilli(nowMillis).Local()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	next := startOfDay.Add(offset)
	if !next.After(now) {
		next = startOfDay.AddDate(0, 0, 1).Add(offset)
	}
	return next.UnixMilli(), true
}

func (g *Gateway) notifyMaintenanceRunner() {
	if g != nil && g.maintenance != nil {
		g.maintenance.notify()
	}
}

func (g *Gateway) enqueueObservationForSubscriptionNodes(subscriptionID string) {
	rows, err := g.db.Query(
		`SELECT n.id, n.name
		   FROM nodes n
		   JOIN node_sources s ON s.node_id = n.id
		  WHERE s.source_id = ? AND s.source_type = 'subscription' AND n.enabled = 1
		  ORDER BY n.created_at, n.id`,
		subscriptionID,
	)
	if err != nil {
		return
	}
	var targets []nodeRecord
	for rows.Next() {
		var nodeID, name string
		if err := rows.Scan(&nodeID, &name); err == nil {
			targets = append(targets, nodeRecord{ID: nodeID, Name: name, Enabled: true})
		}
	}
	if err := rows.Close(); err != nil {
		return
	}
	settings, _ := g.loadMaintenanceSettings()
	plan := appobservations.PlanSubscriptionRefreshAggregateRun(toObservationNodeTargets(targets), settings.EgressIPProbeURL)
	if !plan.CreateRun {
		return
	}
	if _, err := g.createNodeObservationRun(plan.TriggerSource, plan.Scope, toObservationNodeRecords(plan.Targets), plan.ProbeURL); err != nil {
		return
	}
	if plan.NotifyRunner {
		g.notifyMaintenanceRunner()
	}
}
