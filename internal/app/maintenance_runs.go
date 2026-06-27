package app

import (
	"context"
	"errors"

	appevaluations "proxygateway/internal/application/evaluations"
	appgeoip "proxygateway/internal/application/geoip"
	maintenanceapp "proxygateway/internal/application/maintenance"
	appsubscriptions "proxygateway/internal/application/subscriptions"

	"go.uber.org/zap"
)

const (
	maintenanceRunTypeProfileSwitch = "profile_switch"
	maintenanceRunTypeLogCleanup    = maintenanceapp.RunTypeLogCleanup
	maintenanceRunTypeStartup       = maintenanceapp.RunTypeStartupCleanup

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

type maintenanceRunRecord = maintenanceapp.Run

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
	g.log().Debug("maintenance run created",
		zap.String("run_id", run.ID),
		zap.String("run_type", run.RunType),
		zap.String("trigger_source", run.TriggerSource),
		zap.String("target_id", run.TargetID),
		zap.Int("total_count", run.TotalCount),
	)
	return run, nil
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
	return g.maintenanceRunService().CancelUnfinishedNodeObservationAggregateRuns(context.Background(), reasonCode)
}

func (g *Gateway) hasUnfinishedNodeObservationAggregateRun() bool {
	ok, err := g.maintenanceRunService().HasUnfinishedNodeObservationAggregateRun(context.Background())
	if err != nil {
		return false
	}
	return ok
}

func (g *Gateway) claimNextQueuedMaintenanceRunBatch(settings maintenanceSettings) []maintenanceRunRecord {
	for _, item := range maintenanceapp.NextBatchClaimLimits(maintenanceapp.ClaimConcurrency{
		SubscriptionRefresh: settings.SubscriptionConcurrency,
		NodeObservation:     settings.NodeObservationConcurrency,
		ProfileEvaluation:   settings.ProfileEvaluationConcurrency,
		GeoIPUpdate:         settings.GeoIPConcurrency,
	}) {
		runs := g.claimQueuedMaintenanceRunsOfType(item.RunType, item.Limit)
		if len(runs) > 0 {
			return runs
		}
	}
	return nil
}

func (g *Gateway) claimQueuedMaintenanceRunsOfType(runType string, limit int) []maintenanceRunRecord {
	runs, err := g.maintenanceRunService().ClaimQueuedRunsOfType(context.Background(), runType, limit)
	if err != nil {
		g.log().Warn("claim maintenance run failed", zap.String("run_type", runType), zap.Error(err))
		return nil
	}
	return runs
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
	err = maintenanceapp.DispatchRun(run, map[string]maintenanceapp.RunExecutor{
		maintenanceTaskNodeObservation: func(run maintenanceRunRecord) error {
			return g.runNodeObservationMaintenanceRun(run.ID)
		},
		maintenanceTaskProfileEvaluation: func(run maintenanceRunRecord) error {
			return g.runProfileEvaluationMaintenanceRun(run.ID)
		},
		maintenanceTaskSubscriptionRefresh: func(run maintenanceRunRecord) error {
			return g.runSubscriptionRefreshMaintenanceRun(run.ID)
		},
		maintenanceTaskGeoIPUpdate: func(run maintenanceRunRecord) error {
			return g.runGeoIPUpdateMaintenanceRun(run.ID)
		},
		maintenanceRunTypeLogCleanup: func(run maintenanceRunRecord) error {
			return g.runLogCleanupMaintenanceRun(run.ID)
		},
	})
	if errors.Is(err, maintenanceapp.ErrUnknownRunType) {
		err := errors.New("unknown maintenance run type")
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, "unknown_run_type", run.FinishedCount, maintenanceRunDetail(run), err.Error())
		return err
	}
	return err
}

func (g *Gateway) cancelExpiredMaintenanceRunsOnStartup() error {
	result, err := maintenanceapp.StartupCleanupService{Runs: g.maintenanceRunService()}.Execute(context.Background())
	if err != nil {
		return err
	}
	for _, ref := range result.EvaluationRefs {
		_, _ = g.enqueueProfileEvaluationRun(ref.ID, ref.Name, "current_node_observed", 0, true)
	}
	return nil
}

func (g *Gateway) runGeoIPUpdateMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		return err
	}
	if run.State == maintenanceRunStateFinished {
		return nil
	}
	detail := maintenanceRunDetail(run)
	detail["source"] = "MetaCubeX"
	if err := g.startMaintenanceRun(run.ID); err != nil {
		return err
	}
	var updater appgeoip.Updater
	if g.geoIP != nil {
		updater = g.geoIP
	}
	finish, err := appgeoip.UpdateService{
		Updater: updater,
		Status: func() any {
			return g.geoIPStatus()
		},
	}.Execute(context.Background(), run)
	if finish.ID != "" {
		_ = g.finishMaintenanceRun(finish.ID, finish.Result, finish.ReasonCode, finish.FinishedCount, finish.Detail, finish.LastError)
	}
	return err
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
	finish, err := maintenanceapp.LogCleanupService{
		RequestLogs:        g.requestLogRepo,
		MaintenanceHistory: g.maintenanceAuxRepo,
		NowMillis:          unixMillisNow,
	}.Execute(context.Background(), run, maintenanceapp.LogCleanupSettings{
		LogRetentionEnabled:                boolKVSettingDefaultTrue(g.getKVSetting("log_retention_enabled")),
		LogRetentionDays:                   systemSettingPositiveInt(g.getKVSetting("log_retention_days"), defaultRequestLogRetentionDays),
		MaintenanceHistoryRetentionEnabled: boolKVSettingDefaultTrue(g.getKVSetting("maintenance_history_retention_enabled")),
		MaintenanceHistoryRetentionDays:    systemSettingPositiveInt(g.getKVSetting("maintenance_history_retention_days"), defaultMaintenanceHistoryRetentionDays),
	})
	if finish.ID != "" {
		_ = g.finishMaintenanceRun(finish.ID, finish.Result, finish.ReasonCode, finish.FinishedCount, finish.Detail, finish.LastError)
	}
	return err
}

func (g *Gateway) runSubscriptionRefreshMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		return err
	}
	if run.State == maintenanceRunStateFinished {
		return nil
	}
	detail := maintenanceRunDetail(run)
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
		detail = outcome.MaintenanceDetail(detail)
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, outcome.ReasonCode, 1, detail, err.Error())
		return err
	}
	result, err := g.refreshSubscriptionWithContent(sub, content)
	if err != nil {
		outcome := appsubscriptions.BuildRefreshImportFailure(sub.ID, errors.Is(err, errInvalidSubscriptionContent))
		if outcome.PersistLastError {
			g.storeSubscriptionRefreshError(sub.ID, err.Error())
		}
		detail = outcome.MaintenanceDetail(detail)
		_ = g.finishMaintenanceRun(run.ID, maintenanceRunResultFailure, outcome.ReasonCode, 1, detail, err.Error())
		return err
	}
	outcome := appsubscriptions.BuildRefreshSuccessOutcome(appsubscriptions.RefreshImportResultFromImportResult(result))
	detail = outcome.MaintenanceDetail(detail)
	g.logSubscriptionRefreshEntrySummary(run.ID, outcome.SubscriptionID, "ignored", outcome.IgnoredSummary)
	g.logSubscriptionRefreshEntrySummary(run.ID, outcome.SubscriptionID, "skipped", outcome.SkippedSummary)
	g.logSubscriptionRefreshOutcome(run.ID, outcome)
	if outcome.EnqueueObservation {
		g.enqueueObservationForSubscriptionNodes(sub.ID)
	}
	g.enqueueStickyProfileEvaluationsForRemovedNodes(outcome.StickyProfilesToEvaluate)
	return g.finishMaintenanceRun(run.ID, outcome.Result, outcome.ReasonCode, 1, detail, "")
}

func (g *Gateway) runProfileEvaluationMaintenanceRun(runID string) error {
	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		return err
	}
	if run.State == maintenanceRunStateFinished {
		return nil
	}
	detail := maintenanceRunDetail(run)
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
	finish := maintenanceapp.BuildProfileEvaluationFinish(maintenanceapp.ProfileEvaluationFinishInput{
		Detail:           detail,
		EvaluationDetail: parseJSONObject(cfg.LastEvaluationDetailsJSON),
		ProfileID:        run.TargetID,
		ProfileState:     cfg.State,
		CandidateCount:   candidateCount,
		OK:               ok,
		LastError:        cfg.LastError,
		SwitchReason:     cfg.SwitchReason,
	})
	return g.finishMaintenanceRun(run.ID, finish.Result, finish.ReasonCode, finish.FinishedCount, finish.Detail, finish.LastError)
}

func (g *Gateway) profileEvaluationCandidateCount(target profileEvaluationTarget, settings evaluationSettings) int {
	nodes, err := g.candidateNodes(target.Filter)
	if err != nil {
		return 0
	}
	nodeIDs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	return appevaluations.ProfileEvaluationCandidateCount(appevaluations.CandidateCountInput{
		Target:               target,
		CandidateNodeIDs:     nodeIDs,
		SingleCandidateLimit: settings.SingleCandidateLimit,
	})
}

func int64FromDetail(value any) int64 {
	return maintenanceapp.Int64FromDetail(value)
}

func intFromDetail(value any, fallback int) int {
	return maintenanceapp.IntFromDetail(value, fallback)
}

func systemSettingPositiveInt(value string, fallback int) int {
	if parsed := parseInt(value); parsed > 0 {
		return parsed
	}
	return fallback
}

func (g *Gateway) loadMaintenanceRun(id string) (maintenanceRunRecord, error) {
	return g.maintenanceRunService().Load(context.Background(), id)
}

func maintenanceRunDetail(run maintenanceRunRecord) map[string]any {
	return maintenanceapp.RunDetail(run)
}
