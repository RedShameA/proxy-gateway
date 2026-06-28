package app

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appgeoip "proxygateway/internal/application/geoip"
)

func TestManualAllNodeObservationCancelsOnlyAggregateRuns(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	allRun, err := g.createMaintenanceRun(maintenanceTaskNodeObservation, "scheduled", "", "", 2, map[string]any{"target_scope": "all_nodes"})
	if err != nil {
		t.Fatal(err)
	}
	dueRun, err := g.createMaintenanceRun(maintenanceTaskNodeObservation, "scheduled", "", "", 2, map[string]any{"target_scope": "due_nodes"})
	if err != nil {
		t.Fatal(err)
	}
	singleRun, err := g.createMaintenanceRun(maintenanceTaskNodeObservation, "manual", "node_single", "single", 1, map[string]any{"target_scope": "single_node"})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.startMaintenanceRun(dueRun.ID); err != nil {
		t.Fatal(err)
	}

	if err := g.cancelUnfinishedNodeObservationAggregateRuns("replaced_by_manual_run"); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{allRun.ID, dueRun.ID} {
		run, err := g.loadMaintenanceRun(id)
		if err != nil {
			t.Fatal(err)
		}
		if run.State != maintenanceRunStateFinished || run.Result != maintenanceRunResultCancelled || run.ReasonCode != "replaced_by_manual_run" {
			t.Fatalf("aggregate run %s status = %#v, want cancelled replaced_by_manual_run", id, run)
		}
	}
	run, err := g.loadMaintenanceRun(singleRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != maintenanceRunStateQueued || run.Result != "" || run.ReasonCode != "" {
		t.Fatalf("single-node run status = %#v, want still queued without result", run)
	}
}

func TestScheduledNodeObservationCreatesAllNodeRunAndSkipsWhenAggregateUnfinished(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	insertMaintenanceRunTestNode(t, g, "node_all_1", "all-1")
	insertMaintenanceRunTestNode(t, g, "node_all_2", "all-2")
	settings := normalizeMaintenanceSettings(maintenanceSettings{
		NodeObservationSeconds:     1,
		EgressIPProbeURL:           "http://example.invalid/probe",
		NodeObservationConcurrency: 1,
	})

	g.maintenance.enqueueNodeObservationSchedule(100, settings)
	run := latestMaintenanceRunByTypeForTest(t, g, maintenanceTaskNodeObservation)
	if run.TriggerSource != "scheduled" || run.State != maintenanceRunStateQueued || run.TotalCount != 2 {
		t.Fatalf("scheduled all-node run = %#v, want queued scheduled run with total 2", run)
	}
	if maintenanceRunDetail(run)["target_scope"] != "all_nodes" {
		t.Fatalf("scheduled detail = %#v, want all_nodes", maintenanceRunDetail(run))
	}

	g2 := NewForTest(t)
	insertMaintenanceRunTestNode(t, g2, "node_due_blocked", "due-blocked")
	if _, err := g2.createMaintenanceRun(maintenanceTaskNodeObservation, "manual", "", "", 1, map[string]any{"target_scope": "all_nodes"}); err != nil {
		t.Fatal(err)
	}
	g2.maintenance.enqueueNodeObservationSchedule(100, settings)
	skipped := maintenanceRunByReasonForTest(t, g2, "previous_run_still_running")
	if skipped.State != maintenanceRunStateFinished || skipped.Result != maintenanceRunResultSkipped || skipped.ReasonCode != "previous_run_still_running" {
		t.Fatalf("blocked scheduled all-node run = %#v, want skipped previous_run_still_running", skipped)
	}
	if skipped.TotalCount != 1 || skipped.FinishedCount != 0 {
		t.Fatalf("blocked scheduled all-node counts = %d/%d, want 0/1", skipped.FinishedCount, skipped.TotalCount)
	}
}

func TestScheduledNodeObservationRespectsIntervalBetweenAggregateRuns(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	insertMaintenanceRunTestNode(t, g, "node_interval_1", "interval-1")
	settings := normalizeMaintenanceSettings(maintenanceSettings{
		NodeObservationSeconds:     300,
		EgressIPProbeURL:           "http://example.invalid/probe",
		NodeObservationConcurrency: 1,
	})
	now := unixMillisNow()

	g.maintenance.enqueueNodeObservationSchedule(now, settings)
	if count := maintenanceRunCountByTypeForTest(t, g, maintenanceTaskNodeObservation); count != 1 {
		t.Fatalf("scheduled node observation runs = %d, want 1", count)
	}

	g.maintenance.enqueueNodeObservationSchedule(now+secondsToMillis(299), settings)
	if count := maintenanceRunCountByTypeForTest(t, g, maintenanceTaskNodeObservation); count != 1 {
		t.Fatalf("scheduled node observation runs inside interval = %d, want still 1", count)
	}

	g.maintenance.enqueueNodeObservationSchedule(now+secondsToMillis(301), settings)
	if count := maintenanceRunCountByTypeForTest(t, g, maintenanceTaskNodeObservation); count != 2 {
		t.Fatalf("scheduled node observation runs after interval = %d, want 2", count)
	}
	skipped := maintenanceRunByReasonForTest(t, g, "previous_run_still_running")
	if skipped.TriggerSource != "scheduled" || skipped.Result != maintenanceRunResultSkipped {
		t.Fatalf("second scheduled run = %#v, want skipped because first aggregate is unfinished", skipped)
	}
}

func TestScheduledNodeObservationIntervalIgnoresRunHistory(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	insertMaintenanceRunTestNode(t, g, "node_memory_interval", "memory-interval")
	if _, err := g.createMaintenanceRun(maintenanceTaskNodeObservation, "scheduled", "", "", 1, map[string]any{"target_scope": "all_nodes"}); err != nil {
		t.Fatal(err)
	}
	settings := normalizeMaintenanceSettings(maintenanceSettings{
		NodeObservationSeconds:     300,
		EgressIPProbeURL:           "http://example.invalid/probe",
		NodeObservationConcurrency: 1,
	})

	g.maintenance.enqueueNodeObservationSchedule(unixMillisNow(), settings)

	if count := maintenanceRunCountByTypeForTest(t, g, maintenanceTaskNodeObservation); count != 2 {
		t.Fatalf("scheduled node observation runs = %d, want 2 because memory interval state starts empty", count)
	}
	skipped := maintenanceRunByReasonForTest(t, g, "previous_run_still_running")
	if skipped.TriggerSource != "scheduled" || skipped.Result != maintenanceRunResultSkipped {
		t.Fatalf("scheduled run after historical run = %#v, want skipped aggregate conflict", skipped)
	}
}

func TestManualAllNodeObservationDoesNotUpdateScheduledIntervalMemory(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	insertMaintenanceRunTestNode(t, g, "node_manual_memory", "manual-memory")
	settings := normalizeMaintenanceSettings(maintenanceSettings{
		NodeObservationSeconds:     300,
		EgressIPProbeURL:           "http://example.invalid/probe",
		NodeObservationConcurrency: 1,
	})
	g.maintenance.lastScheduledNodeObservationAt = 100

	if _, err := g.createNodeObservationRun("manual", "all_nodes", []nodeRecord{{ID: "node_manual_memory", Name: "manual-memory", Enabled: true}}, settings.EgressIPProbeURL); err != nil {
		t.Fatal(err)
	}
	if g.maintenance.lastScheduledNodeObservationAt != 100 {
		t.Fatalf("manual all-node observation updated scheduled memory to %d, want 100", g.maintenance.lastScheduledNodeObservationAt)
	}

	g.maintenance.enqueueNodeObservationSchedule(100+secondsToMillis(301), settings)
	if count := maintenanceRunCountByTypeForTest(t, g, maintenanceTaskNodeObservation); count != 2 {
		t.Fatalf("scheduled node observation runs after manual all-node run = %d, want 2", count)
	}
}

func TestScheduledNodeObservationIncludesAllEnabledNodes(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	insertMaintenanceRunTestNode(t, g, "node_failed_recent", "failed-recent")
	insertMaintenanceRunTestNode(t, g, "node_failed_stale", "failed-stale")
	insertMaintenanceRunTestNode(t, g, "node_success_recent", "success-recent")
	insertMaintenanceRunTestNode(t, g, "node_never_observed", "never-observed")
	insertMaintenanceRunTestNode(t, g, "node_disabled", "disabled")
	if _, err := g.store.DB.Exec(`UPDATE nodes SET enabled = 0 WHERE id = 'node_disabled'`); err != nil {
		t.Fatal(err)
	}
	settings := normalizeMaintenanceSettings(maintenanceSettings{
		NodeObservationSeconds:     1800,
		EgressIPProbeURL:           "http://example.invalid/probe",
		NodeObservationConcurrency: 1,
	})
	now := int64(10_000_000)
	if _, err := g.store.DB.Exec(
		`INSERT INTO node_observations (node_id, usable, last_error, last_success_at, last_failure_at)
		 VALUES ('node_failed_recent', 0, 'recent failure', 0, ?),
		        ('node_failed_stale', 0, 'stale failure', 0, ?),
		        ('node_success_recent', 1, '', ?, 0)`,
		now-secondsToMillis(60),
		now-secondsToMillis(int64(settings.NodeObservationSeconds))-1,
		now-secondsToMillis(60),
	); err != nil {
		t.Fatal(err)
	}

	g.maintenance.enqueueNodeObservationSchedule(now, settings)

	run := latestMaintenanceRunByTypeForTest(t, g, maintenanceTaskNodeObservation)
	if run.TotalCount != 4 {
		t.Fatalf("scheduled all-node run total = %d, want all 4 enabled nodes", run.TotalCount)
	}
	nodeIDs, ok := maintenanceRunDetail(run)["node_ids"].([]any)
	if !ok {
		t.Fatalf("scheduled node ids = %#v, want array", maintenanceRunDetail(run)["node_ids"])
	}
	got := map[string]bool{}
	for _, nodeID := range nodeIDs {
		text, ok := nodeID.(string)
		if !ok {
			t.Fatalf("scheduled node ids = %#v, want strings", nodeIDs)
		}
		got[text] = true
	}
	for _, nodeID := range []string{"node_failed_recent", "node_failed_stale", "node_success_recent", "node_never_observed"} {
		if !got[nodeID] {
			t.Fatalf("scheduled node ids = %#v, missing %s", nodeIDs, nodeID)
		}
	}
	if got["node_disabled"] {
		t.Fatalf("scheduled node ids = %#v, should exclude disabled node", nodeIDs)
	}
}

func TestQueuedMaintenanceRunsProcessMutatingRefreshBeforeProfileEvaluation(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	settings := normalizeMaintenanceSettings(maintenanceSettings{
		SubscriptionConcurrency:      2,
		NodeObservationConcurrency:   2,
		ProfileEvaluationConcurrency: 2,
		GeoIPConcurrency:             1,
	})
	if _, err := g.createMaintenanceRun(maintenanceTaskNodeObservation, "scheduled", "", "", 1, map[string]any{"target_scope": "all_nodes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.createMaintenanceRun(maintenanceTaskProfileEvaluation, "scheduled", "profile_1", "profile", 1, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.createMaintenanceRun(maintenanceTaskSubscriptionRefresh, "scheduled", "sub_1", "sub", 1, map[string]any{}); err != nil {
		t.Fatal(err)
	}

	runs := g.claimNextQueuedMaintenanceRunBatch(settings)
	if len(runs) != 1 || runs[0].RunType != maintenanceTaskSubscriptionRefresh {
		t.Fatalf("first maintenance batch = %#v, want only subscription_refresh", runs)
	}
	runs = g.claimNextQueuedMaintenanceRunBatch(settings)
	if len(runs) != 1 || runs[0].RunType != maintenanceTaskNodeObservation {
		t.Fatalf("second maintenance batch = %#v, want only node_observation", runs)
	}
	runs = g.claimNextQueuedMaintenanceRunBatch(settings)
	if len(runs) != 1 || runs[0].RunType != maintenanceTaskProfileEvaluation {
		t.Fatalf("third maintenance batch = %#v, want only profile_evaluation", runs)
	}
}

func TestFastestEvaluationDoesNotWriteSelectedNodeDeletedBeforeCommit(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	g.protocolEngine = deletingHTTP200Engine{g: g, deleteNodeID: "node_selected"}
	insertMaintenanceRunTestNode(t, g, "node_selected", "selected")
	cfg := defaultAccessProfileConfig("profile_deleted_selected")
	cfg.Name = "deleted selected"
	cfg.Type = "fastest"
	cfg.TestURL = "http://example.test/models"
	cfg.State = "pending"
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	target, settings, skipped, err := g.profileEvaluationTarget(cfg.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("profile evaluation target skipped")
	}
	settings.GlobalConcurrency = 1

	if ok := g.evaluateFastestProfile(target, settings); ok {
		t.Fatal("evaluation succeeded after selected node was deleted, want failure")
	}
	var state, currentNodeID, switchReason, lastError string
	if err := g.store.DB.QueryRow(`SELECT state, current_node_id, switch_reason, last_error FROM access_profiles WHERE id = ?`, cfg.ID).Scan(&state, &currentNodeID, &switchReason, &lastError); err != nil {
		t.Fatal(err)
	}
	if state != "waiting_observation" || currentNodeID != "" || switchReason != "selected_node_removed" || lastError == "" {
		t.Fatalf("profile after deleted selected node = state=%q current=%q reason=%q err=%q, want waiting_observation empty selected_node_removed", state, currentNodeID, switchReason, lastError)
	}
}

func TestProfileEvaluationRunExecutesFastestProfile(t *testing.T) {
	t.Parallel()

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(probe.Close)

	g := NewForTest(t)
	insertMaintenanceRunTestNode(t, g, "node_fastest", "fastest-node")
	cfg := defaultAccessProfileConfig("profile_fastest_run")
	cfg.Name = "fastest run"
	cfg.Type = "fastest"
	cfg.TestURL = probe.URL
	cfg.MinEvaluationIntervalSeconds = 0
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	runID, err := g.enqueueProfileEvaluationRun(cfg.ID, cfg.Name, "manual", cfg.ConfigVersion, true)
	if err != nil {
		t.Fatal(err)
	}

	if err := g.runMaintenanceRun(runID); err != nil {
		t.Fatal(err)
	}

	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != maintenanceRunStateFinished || run.Result != maintenanceRunResultSuccess {
		t.Fatalf("profile evaluation run status = %#v, want finished success", run)
	}
	if run.TotalCount != 1 || run.FinishedCount != 1 {
		t.Fatalf("profile evaluation counts = %d/%d, want 1/1", run.FinishedCount, run.TotalCount)
	}
	detail := maintenanceRunDetail(run)
	if detail["candidate_count"] != float64(1) || detail["success_count"] != float64(1) || detail["selected_node_id"] != "node_fastest" {
		t.Fatalf("profile evaluation detail = %#v, want candidate/success selected node", detail)
	}
}

func TestProfileEvaluationRunSucceedsWithUsableCandidateDespiteFailures(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	g.protocolEngine = mixedProfileEvaluationEngine{failingNodeID: "node_bad"}
	insertMaintenanceRunTestNode(t, g, "node_ok", "ok-node")
	insertMaintenanceRunTestNode(t, g, "node_bad", "bad-node")
	cfg := defaultAccessProfileConfig("profile_partial_candidates")
	cfg.Name = "partial candidates"
	cfg.Type = "fastest"
	cfg.TestURL = "http://example.test/probe"
	cfg.MinEvaluationIntervalSeconds = 0
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	runID, err := g.enqueueProfileEvaluationRun(cfg.ID, cfg.Name, "manual", cfg.ConfigVersion, true)
	if err != nil {
		t.Fatal(err)
	}

	if err := g.runMaintenanceRun(runID); err != nil {
		t.Fatal(err)
	}

	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != maintenanceRunStateFinished || run.Result != maintenanceRunResultSuccess || run.ReasonCode != "initial_selection" {
		t.Fatalf("profile evaluation run status = %#v, want finished success initial_selection", run)
	}
	if run.TotalCount != 2 || run.FinishedCount != 2 {
		t.Fatalf("profile evaluation counts = %d/%d, want 2/2", run.FinishedCount, run.TotalCount)
	}
	detail := maintenanceRunDetail(run)
	if detail["candidate_count"] != float64(2) || detail["success_count"] != float64(1) || detail["failure_count"] != float64(1) || detail["selected_node_id"] != "node_ok" {
		t.Fatalf("profile evaluation detail = %#v, want candidate=2 success=1 failure=1 selected node_ok", detail)
	}
}

func TestProfileEvaluationRunCancelsWhenSupersededByNewConfigVersion(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	insertMaintenanceRunTestNode(t, g, "node_stale_cfg", "stale-config-node")
	cfg := defaultAccessProfileConfig("profile_stale_cfg")
	cfg.Name = "stale config"
	cfg.Type = "fastest"
	cfg.TestURL = "http://example.test/models"
	cfg.MinEvaluationIntervalSeconds = 0
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	runID, err := g.enqueueProfileEvaluationRun(cfg.ID, cfg.Name, "manual", cfg.ConfigVersion, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.store.DB.Exec(`UPDATE access_profiles SET config_version = ? WHERE id = ?`, cfg.ConfigVersion+1, cfg.ID); err != nil {
		t.Fatal(err)
	}

	if err := g.runMaintenanceRun(runID); err != nil {
		t.Fatal(err)
	}

	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != maintenanceRunStateFinished || run.Result != maintenanceRunResultCancelled || run.ReasonCode != "superseded_by_config_version" {
		t.Fatalf("profile evaluation run status = %#v, want finished cancelled superseded_by_config_version", run)
	}
	if maintenanceRunDetail(run)["current_config_version"] != float64(cfg.ConfigVersion+1) {
		t.Fatalf("run detail = %#v, want current_config_version=%d", maintenanceRunDetail(run), cfg.ConfigVersion+1)
	}
}

func TestStartupCancelsUnfinishedMaintenanceRuns(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	g, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := g.createMaintenanceRun(maintenanceTaskNodeObservation, "manual", "", "", 1, map[string]any{"target_scope": "all_nodes"})
	if err != nil {
		t.Fatal(err)
	}
	running, err := g.createMaintenanceRun(maintenanceTaskProfileEvaluation, "manual", "profile_missing", "missing", 0, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.startMaintenanceRun(running.ID); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	for _, id := range []string{queued.ID, running.ID} {
		run, err := reopened.loadMaintenanceRun(id)
		if err != nil {
			t.Fatal(err)
		}
		if run.State != maintenanceRunStateFinished || run.Result != maintenanceRunResultCancelled || run.ReasonCode != "expired_after_restart" {
			t.Fatalf("run %s after startup = %#v, want cancelled expired_after_restart", id, run)
		}
	}
	startup := startupCleanupRunWithCancelledCountForTest(t, reopened, 2)
	if startup.RunType != maintenanceRunTypeStartup || maintenanceRunDetail(startup)["cancelled_count"] != float64(2) {
		t.Fatalf("startup cleanup run = %#v detail=%#v, want cancelled_count 2", startup, maintenanceRunDetail(startup))
	}
}

func TestStartupRepairsDanglingDynamicProfilePath(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	g, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultAccessProfileConfig("profile_dangling_dynamic")
	cfg.Name = "dangling dynamic"
	cfg.Type = "fastest"
	cfg.CurrentNodeID = "node_missing_current"
	cfg.State = "ready"
	cfg.AutoEvaluationEnabled = true
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	var state, currentNodeID, switchReason string
	if err := reopened.store.DB.QueryRow(`SELECT state, current_node_id, switch_reason FROM access_profiles WHERE id = ?`, cfg.ID).Scan(&state, &currentNodeID, &switchReason); err != nil {
		t.Fatal(err)
	}
	if state != "waiting_observation" || currentNodeID != "" || switchReason != "current_node_removed" {
		t.Fatalf("repaired profile = state=%q current=%q reason=%q, want waiting_observation empty current_node_removed", state, currentNodeID, switchReason)
	}
	startup := latestMaintenanceRunByTypeForTest(t, reopened, maintenanceRunTypeStartup)
	if intFromDetail(maintenanceRunDetail(startup)["repaired_profile_count"], 0) != 1 {
		t.Fatalf("startup cleanup detail = %#v, want repaired_profile_count 1", maintenanceRunDetail(startup))
	}
	var runCount int
	if err := reopened.store.DB.QueryRow(`SELECT COUNT(*) FROM maintenance_runs WHERE run_type = ? AND target_id = ? AND trigger_source = 'current_node_observed'`, maintenanceTaskProfileEvaluation, cfg.ID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("startup cleanup profile evaluation runs = %d, want 1", runCount)
	}
}

func TestGeoIPUpdateRunRecordsFailure(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	g.geoIP = nil
	run, err := g.createMaintenanceRun(maintenanceTaskGeoIPUpdate, "manual", "", "", 1, map[string]any{"source": appgeoip.SourceMetaCubeX})
	if err != nil {
		t.Fatal(err)
	}

	if err := g.runMaintenanceRun(run.ID); err == nil {
		t.Fatal("expected geoip update to fail without geoip service")
	}
	finished, err := g.loadMaintenanceRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != maintenanceRunStateFinished || finished.Result != maintenanceRunResultFailure || finished.ReasonCode != "geoip_service_unavailable" {
		t.Fatalf("geoip run = %#v, want failure geoip_service_unavailable", finished)
	}
	if finished.TargetID != "" || finished.TargetLabel != "" {
		t.Fatalf("geoip target = %q/%q, want empty", finished.TargetID, finished.TargetLabel)
	}
	if maintenanceRunDetail(finished)["source"] != appgeoip.SourceMetaCubeX {
		t.Fatalf("geoip detail = %#v, want source %s", maintenanceRunDetail(finished), appgeoip.SourceMetaCubeX)
	}
	if finished.LastError == "" {
		t.Fatalf("geoip run last_error should be set: %#v", finished)
	}
}

func TestScheduledGeoIPAndLogCleanupRunsUseEmptyTargets(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	now := time.Now().UnixMilli()
	g.maintenance.enqueueGeoIPUpdateSchedule(now, normalizeMaintenanceSettings(maintenanceSettings{
		GeoIPUpdateTime: "00:00",
	}))
	geoIPRun := latestMaintenanceRunByTypeForTest(t, g, maintenanceTaskGeoIPUpdate)
	if geoIPRun.TargetID != "" || geoIPRun.TargetLabel != "" {
		t.Fatalf("scheduled geoip target = %q/%q, want empty", geoIPRun.TargetID, geoIPRun.TargetLabel)
	}
	if maintenanceRunDetail(geoIPRun)["source"] != appgeoip.SourceMetaCubeX {
		t.Fatalf("scheduled geoip detail = %#v, want source %s", maintenanceRunDetail(geoIPRun), appgeoip.SourceMetaCubeX)
	}

	g.maintenance.enqueueLogCleanupSchedule(now)
	logCleanupRun := latestMaintenanceRunByTypeForTest(t, g, maintenanceRunTypeLogCleanup)
	if logCleanupRun.TargetID != "" || logCleanupRun.TargetLabel != "" {
		t.Fatalf("scheduled log cleanup target = %q/%q, want empty", logCleanupRun.TargetID, logCleanupRun.TargetLabel)
	}
}

func TestLogCleanupRunUsesIndependentRequestAndMaintenanceRetention(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	now := time.Now().UnixMilli()
	if _, err := g.store.DB.Exec(
		`INSERT INTO request_logs (id, ts, proxy_credential, access_profile, target_host, proxy_path, success)
		 VALUES ('old_log', ?, 'cred', 'profile', 'old.example:443', 'direct', 0),
		        ('new_log', ?, 'cred', 'profile', 'new.example:443', 'direct', 1)`,
		now-secondsToMillis(3*86400),
		now,
	); err != nil {
		t.Fatal(err)
	}
	oldRun, err := g.createMaintenanceRun(maintenanceTaskNodeObservation, "manual", "", "", 0, map[string]any{"target_scope": "all_nodes"})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.finishMaintenanceRun(oldRun.ID, maintenanceRunResultSuccess, maintenanceRunReasonCompleted, 0, maintenanceRunDetail(oldRun), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := g.store.DB.Exec(`UPDATE maintenance_runs SET created_at = ? WHERE id = ?`, now-secondsToMillis(3*86400), oldRun.ID); err != nil {
		t.Fatal(err)
	}
	g.setKVSetting("log_retention_enabled", "true")
	g.setKVSetting("log_retention_days", "1")
	g.setKVSetting("maintenance_history_retention_enabled", "true")
	g.setKVSetting("maintenance_history_retention_days", "1")
	cleanupRun, err := g.createMaintenanceRun(maintenanceRunTypeLogCleanup, "manual", "", "", 0, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	if err := g.runMaintenanceRun(cleanupRun.ID); err != nil {
		t.Fatal(err)
	}

	var oldLogCount, newLogCount, oldRunCount int
	_ = g.store.DB.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE id = 'old_log'`).Scan(&oldLogCount)
	_ = g.store.DB.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE id = 'new_log'`).Scan(&newLogCount)
	_ = g.store.DB.QueryRow(`SELECT COUNT(*) FROM maintenance_runs WHERE id = ?`, oldRun.ID).Scan(&oldRunCount)
	if oldLogCount != 0 || newLogCount != 1 || oldRunCount != 0 {
		t.Fatalf("cleanup counts oldLog=%d newLog=%d oldRun=%d, want 0/1/0", oldLogCount, newLogCount, oldRunCount)
	}
	finished, err := g.loadMaintenanceRun(cleanupRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.TargetID != "" || finished.TargetLabel != "" {
		t.Fatalf("cleanup target = %q/%q, want empty", finished.TargetID, finished.TargetLabel)
	}
	detail := maintenanceRunDetail(finished)
	if detail["deleted_request_logs"] != float64(1) || detail["deleted_maintenance_runs"] != float64(1) {
		t.Fatalf("cleanup detail = %#v, want deleted_request_logs=1 deleted_maintenance_runs=1", detail)
	}
	if detail["log_retention_enabled"] != true || detail["log_retention_days"] != float64(1) || detail["maintenance_history_retention_enabled"] != true || detail["maintenance_history_retention_days"] != float64(1) {
		t.Fatalf("cleanup retention detail = %#v, want both retention settings enabled with 1 day", detail)
	}
}

func TestLogCleanupRunRespectsRetentionCleanupSwitches(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	now := time.Now().UnixMilli()
	if _, err := g.store.DB.Exec(
		`INSERT INTO request_logs (id, ts, proxy_credential, access_profile, target_host, proxy_path, success)
		 VALUES ('old_log', ?, 'cred', 'profile', 'old.example:443', 'direct', 0)`,
		now-secondsToMillis(3*86400),
	); err != nil {
		t.Fatal(err)
	}
	oldRun, err := g.createMaintenanceRun(maintenanceTaskNodeObservation, "manual", "", "", 0, map[string]any{"target_scope": "all_nodes"})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.finishMaintenanceRun(oldRun.ID, maintenanceRunResultSuccess, maintenanceRunReasonCompleted, 0, maintenanceRunDetail(oldRun), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := g.store.DB.Exec(`UPDATE maintenance_runs SET created_at = ? WHERE id = ?`, now-secondsToMillis(3*86400), oldRun.ID); err != nil {
		t.Fatal(err)
	}
	g.setKVSetting("log_retention_enabled", "false")
	g.setKVSetting("log_retention_days", "1")
	g.setKVSetting("maintenance_history_retention_enabled", "false")
	g.setKVSetting("maintenance_history_retention_days", "1")
	cleanupRun, err := g.createMaintenanceRun(maintenanceRunTypeLogCleanup, "manual", "", "", 0, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}

	if err := g.runMaintenanceRun(cleanupRun.ID); err != nil {
		t.Fatal(err)
	}

	var oldLogCount, oldRunCount int
	_ = g.store.DB.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE id = 'old_log'`).Scan(&oldLogCount)
	_ = g.store.DB.QueryRow(`SELECT COUNT(*) FROM maintenance_runs WHERE id = ?`, oldRun.ID).Scan(&oldRunCount)
	if oldLogCount != 1 || oldRunCount != 1 {
		t.Fatalf("cleanup counts with switches off oldLog=%d oldRun=%d, want 1/1", oldLogCount, oldRunCount)
	}
	finished, err := g.loadMaintenanceRun(cleanupRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	detail := maintenanceRunDetail(finished)
	if detail["deleted_request_logs"] != float64(0) || detail["deleted_maintenance_runs"] != float64(0) {
		t.Fatalf("cleanup detail = %#v, want zero deletions", detail)
	}
	if detail["log_retention_enabled"] != false || detail["maintenance_history_retention_enabled"] != false {
		t.Fatalf("cleanup retention detail = %#v, want both switches disabled", detail)
	}
}

func insertMaintenanceRunTestNode(t *testing.T, g *Gateway, id, name string) {
	t.Helper()
	if _, err := g.store.DB.Exec(
		`INSERT INTO nodes (id, fingerprint, name, type, created_at) VALUES (?, ?, ?, 'direct', ?)`,
		id,
		"fp-"+id,
		name,
		unixMillisNow(),
	); err != nil {
		t.Fatal(err)
	}
}

type maintenanceRunScanner interface {
	Scan(dest ...any) error
}

func scanMaintenanceRun(row maintenanceRunScanner) (maintenanceRunRecord, error) {
	var run maintenanceRunRecord
	var detailJSON string
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
		&detailJSON,
		&run.LastError,
		&run.CreatedAt,
		&run.StartedAt,
		&run.FinishedAt,
		&run.UpdatedAt,
	)
	run.Detail = parseJSONObject(detailJSON)
	return run, err
}

func latestMaintenanceRunByTypeForTest(t *testing.T, g *Gateway, runType string) maintenanceRunRecord {
	t.Helper()
	run, err := scanMaintenanceRun(g.store.DB.QueryRow(
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE run_type = ?
		  ORDER BY created_at DESC, rowid DESC
		  LIMIT 1`,
		runType,
	))
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func maintenanceRunCountByTypeForTest(t *testing.T, g *Gateway, runType string) int {
	t.Helper()
	var count int
	if err := g.store.DB.QueryRow(`SELECT COUNT(*) FROM maintenance_runs WHERE run_type = ?`, runType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func maintenanceRunByReasonForTest(t *testing.T, g *Gateway, reasonCode string) maintenanceRunRecord {
	t.Helper()
	run, err := scanMaintenanceRun(g.store.DB.QueryRow(
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE reason_code = ?
		  LIMIT 1`,
		reasonCode,
	))
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func startupCleanupRunWithCancelledCountForTest(t *testing.T, g *Gateway, cancelledCount int) maintenanceRunRecord {
	t.Helper()
	rows, err := g.store.DB.Query(
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE run_type = ?`,
		maintenanceRunTypeStartup,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		run, err := scanMaintenanceRun(rows)
		if err != nil {
			t.Fatal(err)
		}
		if intFromDetail(maintenanceRunDetail(run)["cancelled_count"], -1) == cancelledCount {
			return run
		}
	}
	t.Fatalf("startup cleanup run with cancelled_count=%d not found", cancelledCount)
	return maintenanceRunRecord{}
}

type deletingHTTP200Engine struct {
	g            *Gateway
	deleteNodeID string
}

func (e deletingHTTP200Engine) DialNode(node nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		_, _ = http.ReadRequest(bufio.NewReader(server))
		if e.g != nil && e.deleteNodeID != "" {
			_, _ = e.g.store.DB.Exec(`DELETE FROM nodes WHERE id = ?`, e.deleteNodeID)
		}
		_, _ = server.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	}()
	return client, nil
}

func (e deletingHTTP200Engine) DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	return e.DialNode(frontNode, target, timeouts)
}

type mixedProfileEvaluationEngine struct {
	failingNodeID string
}

func (e mixedProfileEvaluationEngine) DialNode(node nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	if node.ID == e.failingNodeID {
		return nil, errors.New("candidate dial failed")
	}
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		_, _ = http.ReadRequest(bufio.NewReader(server))
		_, _ = server.Write([]byte("HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n"))
	}()
	return client, nil
}

func (e mixedProfileEvaluationEngine) DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	return e.DialNode(frontNode, target, timeouts)
}
