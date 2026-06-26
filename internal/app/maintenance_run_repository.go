package app

import (
	"context"

	maintenanceapp "proxygateway/internal/application/maintenance"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
)

type maintenanceRunRepository struct {
	sqlite sqliteinfra.MaintenanceRunRepository
}

func (g *Gateway) maintenanceRunService() *maintenanceapp.Service {
	return maintenanceapp.NewService(
		maintenanceRunRepository{sqlite: sqliteinfra.NewMaintenanceRunRepository(g.db)},
		prefixedID,
		unixMillisNow,
	)
}

func (r maintenanceRunRepository) Insert(ctx context.Context, run maintenanceapp.Run) error {
	record, err := sqliteRecordFromApplicationRun(run)
	if err != nil {
		return err
	}
	return r.sqlite.Insert(ctx, record)
}

func (r maintenanceRunRepository) Load(ctx context.Context, id string) (maintenanceapp.Run, error) {
	record, err := r.sqlite.Load(ctx, id)
	if err != nil {
		return maintenanceapp.Run{}, err
	}
	return applicationRunFromSQLiteRecord(record), nil
}

func (r maintenanceRunRepository) Start(ctx context.Context, id string, nowMillis int64) error {
	return r.sqlite.Start(ctx, id, nowMillis)
}

func (r maintenanceRunRepository) SetTotal(ctx context.Context, id string, totalCount int, nowMillis int64) error {
	return r.sqlite.SetTotal(ctx, id, totalCount, nowMillis)
}

func (r maintenanceRunRepository) Finish(ctx context.Context, update maintenanceapp.FinishUpdate) error {
	detailJSON, err := marshalMaintenanceRunDetail(update.Detail)
	if err != nil {
		return err
	}
	return r.sqlite.Finish(ctx, sqliteinfra.MaintenanceRunFinishUpdate{
		ID:            update.ID,
		Result:        update.Result,
		ReasonCode:    update.ReasonCode,
		FinishedCount: update.FinishedCount,
		DetailJSON:    detailJSON,
		LastError:     update.LastError,
		NowMillis:     update.NowMillis,
	})
}

func (r maintenanceRunRepository) ClaimNext(ctx context.Context, runType string, nowMillis int64) (maintenanceapp.Run, bool, error) {
	record, ok, err := r.sqlite.ClaimNext(ctx, runType, nowMillis)
	if err != nil || !ok {
		return maintenanceapp.Run{}, ok, err
	}
	return applicationRunFromSQLiteRecord(record), true, nil
}

func (r maintenanceRunRepository) List(ctx context.Context, filter maintenanceapp.ListFilter) (maintenanceapp.ListResult, error) {
	result, err := r.sqlite.List(ctx, sqliteinfra.MaintenanceRunListFilter{
		RunType:  filter.RunType,
		TargetID: filter.TargetID,
		State:    filter.State,
		Result:   filter.Result,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	})
	if err != nil {
		return maintenanceapp.ListResult{}, err
	}
	items := make([]maintenanceapp.Run, 0, len(result.Items))
	for _, record := range result.Items {
		items = append(items, applicationRunFromSQLiteRecord(record))
	}
	return maintenanceapp.ListResult{
		Items:    items,
		Total:    result.Total,
		Page:     result.Page,
		PageSize: result.PageSize,
	}, nil
}

func applicationRunFromSQLiteRecord(run sqliteinfra.MaintenanceRunRecord) maintenanceapp.Run {
	return maintenanceapp.Run{
		ID:            run.ID,
		RunType:       run.RunType,
		TriggerSource: run.TriggerSource,
		TargetID:      run.TargetID,
		TargetLabel:   run.TargetLabel,
		State:         run.State,
		Result:        run.Result,
		ReasonCode:    run.ReasonCode,
		TotalCount:    run.TotalCount,
		FinishedCount: run.FinishedCount,
		Detail:        parseJSONObject(run.DetailJSON),
		LastError:     run.LastError,
		CreatedAt:     run.CreatedAt,
		StartedAt:     run.StartedAt,
		FinishedAt:    run.FinishedAt,
		UpdatedAt:     run.UpdatedAt,
	}
}

func applicationRunFromRecord(run maintenanceRunRecord) maintenanceapp.Run {
	return maintenanceapp.Run{
		ID:            run.ID,
		RunType:       run.RunType,
		TriggerSource: run.TriggerSource,
		TargetID:      run.TargetID,
		TargetLabel:   run.TargetLabel,
		State:         run.State,
		Result:        run.Result,
		ReasonCode:    run.ReasonCode,
		TotalCount:    run.TotalCount,
		FinishedCount: run.FinishedCount,
		Detail:        run.detail(),
		LastError:     run.LastError,
		CreatedAt:     run.CreatedAt,
		StartedAt:     run.StartedAt,
		FinishedAt:    run.FinishedAt,
		UpdatedAt:     run.UpdatedAt,
	}
}

func sqliteRecordFromApplicationRun(run maintenanceapp.Run) (sqliteinfra.MaintenanceRunRecord, error) {
	detailJSON, err := marshalMaintenanceRunDetail(run.Detail)
	if err != nil {
		return sqliteinfra.MaintenanceRunRecord{}, err
	}
	return sqliteinfra.MaintenanceRunRecord{
		ID:            run.ID,
		RunType:       run.RunType,
		TriggerSource: run.TriggerSource,
		TargetID:      run.TargetID,
		TargetLabel:   run.TargetLabel,
		State:         run.State,
		Result:        run.Result,
		ReasonCode:    run.ReasonCode,
		TotalCount:    run.TotalCount,
		FinishedCount: run.FinishedCount,
		DetailJSON:    detailJSON,
		LastError:     run.LastError,
		CreatedAt:     run.CreatedAt,
		StartedAt:     run.StartedAt,
		FinishedAt:    run.FinishedAt,
		UpdatedAt:     run.UpdatedAt,
	}, nil
}

func recordFromApplicationRun(run maintenanceapp.Run) (maintenanceRunRecord, error) {
	detailJSON, err := marshalMaintenanceRunDetail(run.Detail)
	if err != nil {
		return maintenanceRunRecord{}, err
	}
	return maintenanceRunRecord{
		ID:            run.ID,
		RunType:       run.RunType,
		TriggerSource: run.TriggerSource,
		TargetID:      run.TargetID,
		TargetLabel:   run.TargetLabel,
		State:         run.State,
		Result:        run.Result,
		ReasonCode:    run.ReasonCode,
		TotalCount:    run.TotalCount,
		FinishedCount: run.FinishedCount,
		DetailJSON:    detailJSON,
		LastError:     run.LastError,
		CreatedAt:     run.CreatedAt,
		StartedAt:     run.StartedAt,
		FinishedAt:    run.FinishedAt,
		UpdatedAt:     run.UpdatedAt,
	}, nil
}
