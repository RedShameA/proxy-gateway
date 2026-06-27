package maintenance

import "context"

type RequestLogRetentionRepository interface {
	DeleteBefore(ctx context.Context, cutoffMillis int64) (int64, error)
}

type MaintenanceHistoryRetentionRepository interface {
	DeleteHistoryBefore(ctx context.Context, cutoffMillis int64, keepRunID string) (int64, error)
}

type LogCleanupService struct {
	RequestLogs        RequestLogRetentionRepository
	MaintenanceHistory MaintenanceHistoryRetentionRepository
	NowMillis          func() int64
}

type LogCleanupDetailInput struct {
	Detail                             map[string]any
	DeletedRequestLogs                 int
	DeletedMaintenanceRuns             int
	LogRetentionEnabled                bool
	LogRetentionDays                   int
	MaintenanceHistoryRetentionEnabled bool
	MaintenanceHistoryRetentionDays    int
}

type LogCleanupSettings struct {
	LogRetentionEnabled                bool
	LogRetentionDays                   int
	MaintenanceHistoryRetentionEnabled bool
	MaintenanceHistoryRetentionDays    int
}

func (s LogCleanupService) Execute(ctx context.Context, run Run, settings LogCleanupSettings) (FinishCommand, error) {
	nowMillis := s.nowMillis()
	requestLogDeleted := 0
	if settings.LogRetentionEnabled {
		deleted, err := s.RequestLogs.DeleteBefore(ctx, nowMillis-secondsToMillis(int64(settings.LogRetentionDays*86400)))
		if err != nil {
			return FinishCommand{
				ID:            run.ID,
				Result:        ResultFailure,
				ReasonCode:    ReasonRequestLogCleanupFailed,
				FinishedCount: 0,
				Detail:        RunDetail(run),
				LastError:     err.Error(),
			}, err
		}
		requestLogDeleted = int(deleted)
	}
	maintenanceDeleted := 0
	if settings.MaintenanceHistoryRetentionEnabled {
		deleted, err := s.MaintenanceHistory.DeleteHistoryBefore(ctx, nowMillis-secondsToMillis(int64(settings.MaintenanceHistoryRetentionDays*86400)), run.ID)
		if err != nil {
			return FinishCommand{
				ID:            run.ID,
				Result:        ResultFailure,
				ReasonCode:    ReasonMaintenanceHistoryCleanupFailed,
				FinishedCount: 0,
				Detail:        RunDetail(run),
				LastError:     err.Error(),
			}, err
		}
		maintenanceDeleted = int(deleted)
	}
	return FinishCommand{
		ID:            run.ID,
		Result:        ResultSuccess,
		ReasonCode:    ReasonCompleted,
		FinishedCount: 0,
		Detail: BuildLogCleanupDetail(LogCleanupDetailInput{
			Detail:                             RunDetail(run),
			DeletedRequestLogs:                 requestLogDeleted,
			DeletedMaintenanceRuns:             maintenanceDeleted,
			LogRetentionEnabled:                settings.LogRetentionEnabled,
			LogRetentionDays:                   settings.LogRetentionDays,
			MaintenanceHistoryRetentionEnabled: settings.MaintenanceHistoryRetentionEnabled,
			MaintenanceHistoryRetentionDays:    settings.MaintenanceHistoryRetentionDays,
		}),
	}, nil
}

func (s LogCleanupService) nowMillis() int64 {
	if s.NowMillis != nil {
		return s.NowMillis()
	}
	return 0
}

func BuildLogCleanupDetail(input LogCleanupDetailInput) map[string]any {
	detail := copyDetail(input.Detail)
	detail["deleted_request_logs"] = input.DeletedRequestLogs
	detail["deleted_maintenance_runs"] = input.DeletedMaintenanceRuns
	detail["log_retention_enabled"] = input.LogRetentionEnabled
	detail["log_retention_days"] = input.LogRetentionDays
	detail["maintenance_history_retention_enabled"] = input.MaintenanceHistoryRetentionEnabled
	detail["maintenance_history_retention_days"] = input.MaintenanceHistoryRetentionDays
	return detail
}
