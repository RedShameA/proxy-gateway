package sqlite

import (
	"context"
	"database/sql"
	"strings"
)

const (
	maintenanceRunStateQueued   = "queued"
	maintenanceRunStateRunning  = "running"
	maintenanceRunStateFinished = "finished"
)

type MaintenanceRunRepository struct {
	db *sql.DB
}

type maintenanceRunScanner interface {
	Scan(dest ...any) error
}

type MaintenanceRunRecord struct {
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

type MaintenanceRunFinishUpdate struct {
	ID            string
	Result        string
	ReasonCode    string
	FinishedCount int
	DetailJSON    string
	LastError     string
	NowMillis     int64
}

type MaintenanceRunListFilter struct {
	RunType  string
	TargetID string
	State    string
	Result   string
	Page     int
	PageSize int
}

type MaintenanceRunListResult struct {
	Items    []MaintenanceRunRecord
	Total    int
	Page     int
	PageSize int
}

func NewMaintenanceRunRepository(db *sql.DB) MaintenanceRunRepository {
	return MaintenanceRunRepository{db: db}
}

func (r MaintenanceRunRepository) Insert(ctx context.Context, run MaintenanceRunRecord) error {
	detailJSON := run.DetailJSON
	if strings.TrimSpace(detailJSON) == "" {
		detailJSON = "{}"
	}
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO maintenance_runs (
			id, run_type, trigger_source, target_id, target_label, state, total_count,
			finished_count, detail_json, created_at, updated_at
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		run.ID,
		run.RunType,
		run.TriggerSource,
		run.TargetID,
		run.TargetLabel,
		run.State,
		run.TotalCount,
		detailJSON,
		run.CreatedAt,
		run.UpdatedAt,
	)
	return err
}

func (r MaintenanceRunRepository) Load(ctx context.Context, id string) (MaintenanceRunRecord, error) {
	record, err := scanMaintenanceRun(r.db.QueryRowContext(
		ctx,
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE id = ?`,
		id,
	))
	if err != nil {
		return MaintenanceRunRecord{}, err
	}
	return record, nil
}

func (r MaintenanceRunRepository) Start(ctx context.Context, id string, nowMillis int64) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE maintenance_runs
		    SET state = ?,
		        started_at = CASE WHEN started_at = 0 THEN ? ELSE started_at END,
		        updated_at = ?
		  WHERE id = ? AND state IN (?, ?)`,
		maintenanceRunStateRunning,
		nowMillis,
		nowMillis,
		id,
		maintenanceRunStateQueued,
		maintenanceRunStateRunning,
	)
	return err
}

func (r MaintenanceRunRepository) SetTotal(ctx context.Context, id string, totalCount int, nowMillis int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE maintenance_runs SET total_count = ?, updated_at = ? WHERE id = ?`, totalCount, nowMillis, id)
	return err
}

func (r MaintenanceRunRepository) Finish(ctx context.Context, update MaintenanceRunFinishUpdate) error {
	detailJSON := update.DetailJSON
	if strings.TrimSpace(detailJSON) == "" {
		detailJSON = "{}"
	}
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE maintenance_runs
		    SET state = ?,
		        result = ?,
		        reason_code = ?,
		        finished_count = ?,
		        detail_json = ?,
		        last_error = ?,
		        finished_at = ?,
		        updated_at = ?
		  WHERE id = ?`,
		maintenanceRunStateFinished,
		update.Result,
		update.ReasonCode,
		update.FinishedCount,
		detailJSON,
		update.LastError,
		update.NowMillis,
		update.NowMillis,
		update.ID,
	)
	return err
}

func (r MaintenanceRunRepository) ClaimNext(ctx context.Context, runType string, nowMillis int64) (MaintenanceRunRecord, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return MaintenanceRunRecord{}, false, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	var runID string
	err = tx.QueryRowContext(
		ctx,
		`SELECT id
		   FROM maintenance_runs
		  WHERE state = ? AND run_type = ?
		  ORDER BY created_at ASC, rowid ASC
		  LIMIT 1`,
		maintenanceRunStateQueued,
		runType,
	).Scan(&runID)
	if err != nil {
		return MaintenanceRunRecord{}, false, nil
	}
	res, err := tx.ExecContext(
		ctx,
		`UPDATE maintenance_runs
		    SET state = ?,
		        started_at = CASE WHEN started_at = 0 THEN ? ELSE started_at END,
		        updated_at = ?
		  WHERE id = ? AND state = ?`,
		maintenanceRunStateRunning,
		nowMillis,
		nowMillis,
		runID,
		maintenanceRunStateQueued,
	)
	if err != nil {
		return MaintenanceRunRecord{}, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return MaintenanceRunRecord{}, false, err
	}
	record, err := scanMaintenanceRun(tx.QueryRowContext(
		ctx,
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs WHERE id = ?`,
		runID,
	))
	if err != nil {
		return MaintenanceRunRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return MaintenanceRunRecord{}, false, err
	}
	committed = true
	return record, true, nil
}

func (r MaintenanceRunRepository) List(ctx context.Context, filter MaintenanceRunListFilter) (MaintenanceRunListResult, error) {
	where, args := maintenanceRunListWhere(filter)
	var total int
	countArgs := append([]any{}, args...)
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM maintenance_runs `+where, countArgs...).Scan(&total); err != nil {
		return MaintenanceRunListResult{}, err
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs `+where+`
		  ORDER BY created_at DESC, rowid DESC
		  LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		return MaintenanceRunListResult{}, err
	}
	defer rows.Close()
	items := []MaintenanceRunRecord{}
	for rows.Next() {
		record, err := scanMaintenanceRun(rows)
		if err != nil {
			return MaintenanceRunListResult{}, err
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return MaintenanceRunListResult{}, err
	}
	return MaintenanceRunListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func maintenanceRunListWhere(filter MaintenanceRunListFilter) (string, []any) {
	var clauses []string
	var args []any
	for _, item := range []struct {
		value string
		field string
	}{
		{value: filter.RunType, field: "run_type"},
		{value: filter.TargetID, field: "target_id"},
		{value: filter.State, field: "state"},
		{value: filter.Result, field: "result"},
	} {
		value := strings.TrimSpace(item.value)
		if value == "" {
			continue
		}
		clauses = append(clauses, item.field+" = ?")
		args = append(args, value)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func scanMaintenanceRun(row maintenanceRunScanner) (MaintenanceRunRecord, error) {
	var run MaintenanceRunRecord
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
