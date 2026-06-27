package app

import (
	"context"
	"sync"
	"time"

	appmaintenance "proxygateway/internal/application/maintenance"
	appobservations "proxygateway/internal/application/observations"
	appsettings "proxygateway/internal/application/settings"

	"go.uber.org/zap"
)

const (
	maintenanceTaskSubscriptionRefresh = appmaintenance.RunTypeSubscriptionRefresh
	maintenanceTaskNodeObservation     = appmaintenance.RunTypeNodeObservation
	maintenanceTaskProfileEvaluation   = appmaintenance.RunTypeProfileEvaluation
	maintenanceTaskGeoIPUpdate         = appmaintenance.RunTypeGeoIPUpdate
)

type maintenanceRunner struct {
	g                              *Gateway
	runner                         *appmaintenance.Runner
	workerMu                       sync.Mutex
	scheduleMu                     sync.Mutex
	lastScheduledNodeObservationAt int64
}

func newMaintenanceRunner(g *Gateway) *maintenanceRunner {
	r := &maintenanceRunner{g: g}
	r.runner = appmaintenance.NewRunner(appmaintenance.RunnerCallbacks{
		Context: g.ctx,
		OnStarted: func() {
			g.log().Info("maintenance runner started")
		},
		OnStopped: func() {
			g.log().Info("maintenance runner stopped")
		},
		OnWake: func() {
			g.log().Debug("maintenance runner woke")
		},
		OnTick: func() {
			g.log().Debug("maintenance runner tick")
		},
		EnqueueDue: r.enqueueDueScheduledTasks,
		RunQueued:  r.runQueuedTasks,
	})
	return r
}

func (r *maintenanceRunner) start() {
	if r == nil || r.g == nil {
		return
	}
	r.runner.Start()
}

func (r *maintenanceRunner) notify() {
	if r == nil {
		return
	}
	r.runner.Notify()
}

func (r *maintenanceRunner) enqueueDueScheduledTasks() {
	settings, err := r.g.loadMaintenanceSettings()
	if err != nil {
		r.g.log().Warn("load maintenance settings failed", zap.Error(err))
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
	scheduleTargets, err := r.g.maintenanceAuxRepo.ListNodeObservationScheduleTargets(context.Background())
	if err != nil {
		r.g.log().Warn("query node observation schedule targets failed", zap.Error(err))
		return
	}
	targets := nodeRecordsFromObservationScheduleTargets(scheduleTargets)
	if !r.reserveScheduledNodeObservation(now, settings.NodeObservationSeconds) {
		return
	}
	plan := appobservations.PlanScheduledAggregateRun(
		toObservationNodeTargets(targets),
		settings.EgressIPProbeURL,
		r.g.hasUnfinishedNodeObservationAggregateRun(),
	)
	if !plan.CreateRun {
		r.g.log().Debug("node observation schedule skipped",
			zap.String("reason", plan.ReasonCode),
			zap.String("scope", plan.Scope),
		)
		return
	}
	run, err := r.g.createNodeObservationRun(plan.TriggerSource, plan.Scope, toObservationNodeRecords(plan.Targets), plan.ProbeURL)
	if err != nil {
		r.g.log().Warn("create node observation run failed",
			zap.String("trigger_source", plan.TriggerSource),
			zap.String("scope", plan.Scope),
			zap.Error(err),
		)
		return
	}
	r.g.log().Info("maintenance run queued",
		zap.String("run_id", run.ID),
		zap.String("run_type", run.RunType),
		zap.String("trigger_source", run.TriggerSource),
		zap.Int("total_count", run.TotalCount),
	)
	if plan.FinishImmediately {
		detail := maintenanceRunDetail(run)
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
	scheduleTargets, err := r.g.maintenanceAuxRepo.ListProfileEvaluationScheduleTargets(context.Background())
	if err != nil {
		r.g.log().Warn("query profile evaluation schedule targets failed", zap.Error(err))
		return
	}
	targets := appmaintenance.DueProfileEvaluationTargets(scheduleTargets, now, settings.ProfileEvaluationSeconds, settings.ChainEvaluationSeconds)
	for _, target := range targets {
		if runID, err := r.g.enqueueProfileEvaluationRun(target.ID, target.Name, "scheduled", target.ConfigVersion, false); err != nil {
			r.g.log().Warn("enqueue profile evaluation run failed",
				zap.String("profile_id", target.ID),
				zap.String("profile_name", target.Name),
				zap.Error(err),
			)
		} else {
			r.g.log().Info("maintenance run queued",
				zap.String("run_id", runID),
				zap.String("run_type", maintenanceTaskProfileEvaluation),
				zap.String("trigger_source", "scheduled"),
			)
		}
	}
}

func (r *maintenanceRunner) enqueueSubscriptionRefreshSchedule(now int64, settings maintenanceSettings) {
	scheduleTargets, err := r.g.maintenanceAuxRepo.ListSubscriptionRefreshScheduleTargets(context.Background())
	if err != nil {
		r.g.log().Warn("query subscription refresh schedule targets failed", zap.Error(err))
		return
	}
	targets := appmaintenance.DueSubscriptionRefreshTargets(scheduleTargets, now, settings.SubscriptionRefreshSeconds)
	for _, target := range targets {
		if runID, err := r.g.enqueueSubscriptionRefreshRun(target.ID, target.Name, "scheduled"); err != nil {
			r.g.log().Warn("enqueue subscription refresh run failed",
				zap.String("subscription_id", target.ID),
				zap.String("subscription_name", target.Name),
				zap.Error(err),
			)
		} else {
			r.g.log().Info("maintenance run queued",
				zap.String("run_id", runID),
				zap.String("run_type", maintenanceTaskSubscriptionRefresh),
				zap.String("trigger_source", "scheduled"),
			)
		}
	}
}

func (r *maintenanceRunner) enqueueGeoIPUpdateSchedule(now int64, settings maintenanceSettings) {
	if r == nil || r.g == nil || r.g.geoIP == nil {
		return
	}
	status, _ := r.g.geoIPStatusRepo.LoadStatus(context.Background())
	if !appmaintenance.ShouldQueueGeoIPUpdate(now, settings.GeoIPUpdateTime, appmaintenance.GeoIPScheduleStatus{
		LoadedAt:  status.LoadedAt,
		UpdatedAt: status.UpdatedAt,
	}) {
		return
	}
	run, err := r.g.createMaintenanceRun(maintenanceTaskGeoIPUpdate, "scheduled", "country.mmdb", "GeoIP Database", 1, map[string]any{
		"source": "MetaCubeX",
	})
	if err != nil {
		r.g.log().Warn("enqueue geoip update run failed", zap.Error(err))
		return
	}
	r.g.log().Info("maintenance run queued",
		zap.String("run_id", run.ID),
		zap.String("run_type", run.RunType),
		zap.String("trigger_source", run.TriggerSource),
	)
	r.g.notifyMaintenanceRunner()
}

func (r *maintenanceRunner) enqueueLogCleanupSchedule(now int64) {
	recent, err := r.g.maintenanceAuxRepo.HasRecentRun(context.Background(), maintenanceRunTypeLogCleanup, now-secondsToMillis(86400))
	if err != nil {
		r.g.log().Warn("query recent log cleanup run failed", zap.Error(err))
		return
	}
	if recent {
		return
	}
	run, err := r.g.createMaintenanceRun(maintenanceRunTypeLogCleanup, "scheduled", "", "Retention cleanup", 0, map[string]any{})
	if err != nil {
		r.g.log().Warn("enqueue log cleanup run failed", zap.Error(err))
		return
	}
	r.g.log().Info("maintenance run queued",
		zap.String("run_id", run.ID),
		zap.String("run_type", run.RunType),
		zap.String("trigger_source", run.TriggerSource),
	)
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
			r.g.log().Warn("load maintenance settings failed", zap.Error(err))
			return
		}
		runs := r.g.claimNextQueuedMaintenanceRunBatch(settings)
		if len(runs) == 0 {
			return
		}
		r.g.log().Info("maintenance run batch claimed", zap.Int("count", len(runs)), zap.String("run_type", runs[0].RunType))
		var wg sync.WaitGroup
		for _, run := range runs {
			run := run
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := r.g.runMaintenanceRun(run.ID); err != nil {
					r.g.log().Warn("maintenance run failed",
						zap.String("run_id", run.ID),
						zap.String("run_type", run.RunType),
						zap.String("trigger_source", run.TriggerSource),
						zap.Error(err),
					)
				}
			}()
		}
		wg.Wait()
	}
}

func (g *Gateway) loadMaintenanceSettings() (maintenanceSettings, error) {
	settings, err := g.systemSettingsRepo.LoadMaintenance(context.Background())
	return normalizeMaintenanceSettings(maintenanceSettingsFromApplication(settings)), err
}

func normalizeMaintenanceSettings(settings maintenanceSettings) maintenanceSettings {
	return maintenanceSettingsFromApplication(appsettings.NormalizeMaintenance(applicationMaintenanceSettings(settings)))
}

func validateMaintenanceSettings(settings maintenanceSettings) error {
	return appsettings.ValidateMaintenance(applicationMaintenanceSettings(settings))
}

func (g *Gateway) saveMaintenanceSettings(settings maintenanceSettings) error {
	return g.systemSettingsRepo.SaveMaintenance(context.Background(), applicationMaintenanceSettings(settings))
}

func maintenanceSettingsFromApplication(settings appsettings.MaintenanceSettings) maintenanceSettings {
	return maintenanceSettings{
		SubscriptionRefreshSeconds:   settings.SubscriptionRefreshSeconds,
		NodeObservationSeconds:       settings.NodeObservationSeconds,
		ProfileEvaluationSeconds:     settings.ProfileEvaluationSeconds,
		ChainEvaluationSeconds:       settings.ChainEvaluationSeconds,
		GeoIPUpdateTime:              settings.GeoIPUpdateTime,
		EgressIPProbeURL:             settings.EgressIPProbeURL,
		SubscriptionConcurrency:      settings.SubscriptionConcurrency,
		NodeObservationConcurrency:   settings.NodeObservationConcurrency,
		ProfileEvaluationConcurrency: settings.ProfileEvaluationConcurrency,
		GeoIPConcurrency:             settings.GeoIPConcurrency,
	}
}

func applicationMaintenanceSettings(settings maintenanceSettings) appsettings.MaintenanceSettings {
	return appsettings.MaintenanceSettings{
		SubscriptionRefreshSeconds:   settings.SubscriptionRefreshSeconds,
		NodeObservationSeconds:       settings.NodeObservationSeconds,
		ProfileEvaluationSeconds:     settings.ProfileEvaluationSeconds,
		ChainEvaluationSeconds:       settings.ChainEvaluationSeconds,
		GeoIPUpdateTime:              settings.GeoIPUpdateTime,
		EgressIPProbeURL:             settings.EgressIPProbeURL,
		SubscriptionConcurrency:      settings.SubscriptionConcurrency,
		NodeObservationConcurrency:   settings.NodeObservationConcurrency,
		ProfileEvaluationConcurrency: settings.ProfileEvaluationConcurrency,
		GeoIPConcurrency:             settings.GeoIPConcurrency,
	}
}

func (g *Gateway) notifyMaintenanceRunner() {
	if g != nil && g.maintenance != nil {
		g.maintenance.notify()
	}
}

func (g *Gateway) enqueueObservationForSubscriptionNodes(subscriptionID string) {
	targets, err := g.maintenanceAuxRepo.ListSubscriptionNodeObservationTargets(context.Background(), subscriptionID)
	if err != nil {
		return
	}
	settings, _ := g.loadMaintenanceSettings()
	plan := appobservations.PlanSubscriptionRefreshAggregateRun(toObservationNodeTargets(nodeRecordsFromObservationScheduleTargets(targets)), settings.EgressIPProbeURL)
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

func nodeRecordsFromObservationScheduleTargets(targets []appmaintenance.NodeObservationScheduleTarget) []nodeRecord {
	nodes := make([]nodeRecord, 0, len(targets))
	for _, target := range targets {
		nodes = append(nodes, nodeRecord{ID: target.ID, Name: target.Name, Enabled: true})
	}
	return nodes
}
