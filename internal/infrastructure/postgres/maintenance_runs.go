package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	appevaluations "proxygateway/internal/application/evaluations"
	maintenanceapp "proxygateway/internal/application/maintenance"
	domainprofile "proxygateway/internal/domain/profile"
)

const (
	maintenanceRunStateQueued   = maintenanceapp.StateQueued
	maintenanceRunStateRunning  = maintenanceapp.StateRunning
	maintenanceRunStateFinished = maintenanceapp.StateFinished
)

type MaintenanceRunRepository struct {
	db *sql.DB
}

type maintenanceRunScanner interface {
	Scan(dest ...any) error
}

func NewMaintenanceRunRepository(db *sql.DB) MaintenanceRunRepository {
	return MaintenanceRunRepository{db: db}
}

func (r MaintenanceRunRepository) Insert(ctx context.Context, run maintenanceapp.Run) error {
	detailJSON, err := maintenanceRunDetailJSON(run.Detail)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		`INSERT INTO maintenance_runs (
			id, run_type, trigger_source, target_id, target_label, state, total_count,
			finished_count, detail_json, created_at, updated_at
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, 0, $8, $9, $10)`,
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

func (r MaintenanceRunRepository) Load(ctx context.Context, id string) (maintenanceapp.Run, error) {
	run, err := scanMaintenanceRun(r.db.QueryRowContext(
		ctx,
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE id = $1`,
		id,
	))
	if err == sql.ErrNoRows {
		return maintenanceapp.Run{}, maintenanceapp.ErrRunNotFound
	}
	return run, err
}

func (r MaintenanceRunRepository) Start(ctx context.Context, id string, nowMillis int64) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE maintenance_runs
		    SET state = $1,
		        started_at = CASE WHEN started_at = 0 THEN $2 ELSE started_at END,
		        updated_at = $3
		  WHERE id = $4 AND state IN ($5, $6)`,
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
	_, err := r.db.ExecContext(ctx, `UPDATE maintenance_runs SET total_count = $1, updated_at = $2 WHERE id = $3`, totalCount, nowMillis, id)
	return err
}

func (r MaintenanceRunRepository) Finish(ctx context.Context, update maintenanceapp.FinishUpdate) error {
	detailJSON, err := maintenanceRunDetailJSON(update.Detail)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(
		ctx,
		`UPDATE maintenance_runs
		    SET state = $1,
		        result = $2,
		        reason_code = $3,
		        finished_count = $4,
		        detail_json = $5,
		        last_error = $6,
		        finished_at = $7,
		        updated_at = $8
		  WHERE id = $9`,
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

func (r MaintenanceRunRepository) ClaimNext(ctx context.Context, runType string, nowMillis int64) (maintenanceapp.Run, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return maintenanceapp.Run{}, false, err
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
		  WHERE state = $1 AND run_type = $2
		  ORDER BY created_at ASC, sequence ASC
		  LIMIT 1
		  FOR UPDATE SKIP LOCKED`,
		maintenanceRunStateQueued,
		runType,
	).Scan(&runID)
	if err == sql.ErrNoRows {
		return maintenanceapp.Run{}, false, nil
	}
	if err != nil {
		return maintenanceapp.Run{}, false, err
	}
	res, err := tx.ExecContext(
		ctx,
		`UPDATE maintenance_runs
		    SET state = $1,
		        started_at = CASE WHEN started_at = 0 THEN $2 ELSE started_at END,
		        updated_at = $3
		  WHERE id = $4 AND state = $5`,
		maintenanceRunStateRunning,
		nowMillis,
		nowMillis,
		runID,
		maintenanceRunStateQueued,
	)
	if err != nil {
		return maintenanceapp.Run{}, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return maintenanceapp.Run{}, false, err
	}
	run, err := scanMaintenanceRun(tx.QueryRowContext(
		ctx,
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs WHERE id = $1`,
		runID,
	))
	if err != nil {
		return maintenanceapp.Run{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return maintenanceapp.Run{}, false, err
	}
	committed = true
	return run, true, nil
}

func (r MaintenanceRunRepository) List(ctx context.Context, filter maintenanceapp.ListFilter) (maintenanceapp.ListResult, error) {
	where, args := maintenanceRunListWhere(filter)
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM maintenance_runs `+where, args...).Scan(&total); err != nil {
		return maintenanceapp.ListResult{}, err
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
		  ORDER BY created_at DESC, sequence DESC
		  LIMIT $`+placeholder(len(args)-1)+` OFFSET $`+placeholder(len(args)),
		args...,
	)
	if err != nil {
		return maintenanceapp.ListResult{}, err
	}
	items, err := scanMaintenanceRuns(rows)
	if err != nil {
		return maintenanceapp.ListResult{}, err
	}
	return maintenanceapp.ListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r MaintenanceRunRepository) ListProfileEvents(ctx context.Context, profileID string, limit int) ([]maintenanceapp.Run, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE target_id = $1 AND run_type IN ('profile_evaluation', 'profile_switch')
		  ORDER BY created_at DESC, sequence DESC LIMIT $2`,
		profileID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	return scanMaintenanceRuns(rows)
}

func (r MaintenanceRunRepository) ListUnfinished(ctx context.Context, runType string) ([]maintenanceapp.Run, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE run_type = $1 AND state != $2
		  ORDER BY created_at ASC, sequence ASC`,
		runType,
		maintenanceRunStateFinished,
	)
	if err != nil {
		return nil, err
	}
	return scanMaintenanceRuns(rows)
}

func (r MaintenanceRunRepository) ListActive(ctx context.Context) ([]maintenanceapp.Run, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  WHERE state IN ($1, $2)
		  ORDER BY created_at ASC, sequence ASC`,
		maintenanceRunStateQueued,
		maintenanceRunStateRunning,
	)
	if err != nil {
		return nil, err
	}
	return scanMaintenanceRuns(rows)
}

func (r MaintenanceRunRepository) RepairDanglingProfilePaths(ctx context.Context, nowMillis int64) (maintenanceapp.DanglingProfileRepairResult, error) {
	rows, err := r.db.QueryContext(
		ctx,
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
		return maintenanceapp.DanglingProfileRepairResult{}, err
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
		if err := rows.Scan(&profile.id, &profile.name, &profile.profileType, &profile.fixedNodeID, &profile.currentNode, &profile.currentExit, &profile.autoEval); err != nil {
			_ = rows.Close()
			return maintenanceapp.DanglingProfileRepairResult{}, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return maintenanceapp.DanglingProfileRepairResult{}, err
	}
	if err := rows.Close(); err != nil {
		return maintenanceapp.DanglingProfileRepairResult{}, err
	}
	result := maintenanceapp.DanglingProfileRepairResult{}
	for _, profile := range profiles {
		if profile.fixedNodeID != "" && r.nodeMissing(ctx, profile.fixedNodeID) {
			if _, err := r.db.ExecContext(
				ctx,
				`UPDATE access_profiles
				    SET current_node_id = '',
				        current_exit_node_id = '',
				        state = $1,
				        last_error = 'referenced node no longer exists',
				        switch_reason = $2,
				        last_evaluated_at = $3
				  WHERE id = $4`,
				appevaluations.ProfileStateInvalidConfig,
				appevaluations.SwitchReasonMissingFixedNode,
				nowMillis,
				profile.id,
			); err != nil {
				return result, err
			}
			result.InvalidCount++
			continue
		}
		if profile.profileType != domainprofile.TypeFastest && profile.profileType != domainprofile.TypeChain {
			continue
		}
		if profile.autoEval {
			if _, err := r.db.ExecContext(
				ctx,
				`UPDATE access_profiles
				    SET current_node_id = '',
				        current_exit_node_id = '',
				        state = $1,
				        last_error = '',
				        switch_reason = $2,
				        last_evaluation_started_at = $3
				  WHERE id = $4`,
				appevaluations.ProfileStateWaitingObservation,
				appevaluations.SwitchReasonCurrentNodeRemoved,
				nowMillis,
				profile.id,
			); err != nil {
				return result, err
			}
			result.EvaluationRefs = append(result.EvaluationRefs, maintenanceapp.ProfileEvaluationRef{ID: profile.id, Name: profile.name})
		} else {
			if _, err := r.db.ExecContext(
				ctx,
				`UPDATE access_profiles
				    SET current_node_id = '',
				        current_exit_node_id = '',
				        state = $1,
				        last_error = 'current node no longer exists',
				        switch_reason = $2
				  WHERE id = $3`,
				appevaluations.ProfileStatePending,
				appevaluations.SwitchReasonCurrentNodeRemoved,
				profile.id,
			); err != nil {
				return result, err
			}
		}
		result.RepairedCount++
	}
	return result, nil
}

func (r MaintenanceRunRepository) nodeMissing(ctx context.Context, nodeID string) bool {
	if strings.TrimSpace(nodeID) == "" {
		return false
	}
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM nodes WHERE id = $1`, nodeID).Scan(&exists)
	return err != nil
}

func maintenanceRunListWhere(filter maintenanceapp.ListFilter) (string, []any) {
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
		args = append(args, value)
		clauses = append(clauses, item.field+" = $"+placeholder(len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

func scanMaintenanceRuns(rows *sql.Rows) ([]maintenanceapp.Run, error) {
	defer rows.Close()
	var runs []maintenanceapp.Run
	for rows.Next() {
		run, err := scanMaintenanceRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func scanMaintenanceRun(row maintenanceRunScanner) (maintenanceapp.Run, error) {
	var run maintenanceapp.Run
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
	if err != nil {
		return maintenanceapp.Run{}, err
	}
	run.Detail = parseMaintenanceRunDetail(detailJSON)
	return run, nil
}

func maintenanceRunDetailJSON(detail map[string]any) (string, error) {
	if len(detail) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(detail)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(data)) == "" {
		return "{}", nil
	}
	return string(data), nil
}

func parseMaintenanceRunDetail(detailJSON string) map[string]any {
	detailJSON = strings.TrimSpace(detailJSON)
	if detailJSON == "" {
		return map[string]any{}
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(detailJSON), &detail); err != nil || detail == nil {
		return map[string]any{}
	}
	return detail
}

func placeholder(index int) string {
	return strconv.Itoa(index)
}

var _ maintenanceapp.Repository = MaintenanceRunRepository{}
