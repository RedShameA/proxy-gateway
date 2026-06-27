package sqlite

import (
	"context"
	"database/sql"
	"strings"

	appproxy "proxygateway/internal/application/proxy"
)

type RequestLogRepository struct {
	db *sql.DB
}

type requestLogScanner interface {
	Scan(dest ...any) error
}

func NewRequestLogRepository(db *sql.DB) RequestLogRepository {
	return RequestLogRepository{db: db}
}

func (r RequestLogRepository) InsertStart(ctx context.Context, record appproxy.RequestLogStartRecord) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO request_logs (
			id, ts, proxy_credential_id, proxy_credential, access_profile_id, access_profile, access_profile_identifier,
			target_host, proxy_path, proxy_path_json, state, success, failure_stage, error, duration_ms, ingress_bytes, egress_bytes, http_status
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, '', '', 0, 0, 0, 0)`,
		record.ID,
		record.Timestamp,
		record.ProxyCredentialID,
		record.ProxyCredential,
		record.AccessProfileID,
		record.AccessProfile,
		record.AccessProfileIdentifier,
		record.TargetHost,
		record.ProxyPath,
		record.ProxyPathJSON,
		appproxy.RequestLogStateRunning,
	)
	return err
}

func (r RequestLogRepository) Finish(ctx context.Context, record appproxy.RequestLogFinishRecord) error {
	successInt := 0
	if record.Success {
		successInt = 1
	}
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE request_logs
		    SET state = ?,
		        success = ?,
		        failure_stage = ?,
		        error = ?,
		        duration_ms = ?,
		        ingress_bytes = ?,
		        egress_bytes = ?,
		        http_status = ?
		  WHERE id = ?`,
		appproxy.RequestLogStateCompleted,
		successInt,
		record.FailureStage,
		record.Error,
		record.DurationMS,
		record.IngressBytes,
		record.EgressBytes,
		record.HTTPStatus,
		record.ID,
	)
	return err
}

func (r RequestLogRepository) InsertFailure(ctx context.Context, record appproxy.RequestLogFailureRecord) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO request_logs (
			id, ts, proxy_credential_id, proxy_credential, access_profile_id, access_profile, access_profile_identifier,
			target_host, proxy_path, proxy_path_json, state, success, failure_stage, error, duration_ms, ingress_bytes, egress_bytes, http_status
		 ) VALUES (?, ?, '', '', '', ?, ?, ?, '', '', ?, 0, ?, ?, ?, 0, 0, ?)`,
		record.ID,
		record.Timestamp,
		record.AccessProfile,
		record.AccessProfileIdentifier,
		record.TargetHost,
		appproxy.RequestLogStateCompleted,
		record.FailureStage,
		record.Error,
		record.DurationMS,
		record.HTTPStatus,
	)
	return err
}

func (r RequestLogRepository) List(ctx context.Context, filter appproxy.RequestLogListFilter) (appproxy.RequestLogListResult, error) {
	where, args := requestLogListWhere(filter)
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_logs`+where, args...).Scan(&total); err != nil {
		return appproxy.RequestLogListResult{}, err
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
	queryArgs := append(append([]any{}, args...), pageSize, offset)
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, ts, proxy_credential_id, proxy_credential, access_profile_id, access_profile, access_profile_identifier,
		        target_host, proxy_path, proxy_path_json, state, success, failure_stage, error, duration_ms, ingress_bytes, egress_bytes, http_status
		   FROM request_logs`+where+`
		  ORDER BY ts DESC, id DESC
		  LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return appproxy.RequestLogListResult{}, err
	}
	defer rows.Close()
	items := []appproxy.RequestLogEntry{}
	for rows.Next() {
		item, err := scanRequestLog(rows)
		if err != nil {
			return appproxy.RequestLogListResult{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return appproxy.RequestLogListResult{}, err
	}
	return appproxy.RequestLogListResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (r RequestLogRepository) CountSince(ctx context.Context, cutoffMillis int64) (appproxy.RequestLogCounts, error) {
	var counts appproxy.RequestLogCounts
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_logs WHERE ts > ?`, cutoffMillis).Scan(&counts.Total); err != nil {
		return appproxy.RequestLogCounts{}, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_logs WHERE ts > ? AND state = ? AND success = 0`, cutoffMillis, appproxy.RequestLogStateCompleted).Scan(&counts.Failed); err != nil {
		return appproxy.RequestLogCounts{}, err
	}
	return counts, nil
}

func (r RequestLogRepository) ListRecentFailures(ctx context.Context, limit int) ([]appproxy.RequestLogEntry, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, ts, proxy_credential_id, proxy_credential, access_profile_id, access_profile, access_profile_identifier,
		        target_host, proxy_path, proxy_path_json, state, success, failure_stage, error, duration_ms, ingress_bytes, egress_bytes, http_status
		   FROM request_logs
		  WHERE state = ? AND success = 0
		  ORDER BY ts DESC
		  LIMIT ?`,
		appproxy.RequestLogStateCompleted,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []appproxy.RequestLogEntry{}
	for rows.Next() {
		item, err := scanRequestLog(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r RequestLogRepository) DeleteBefore(ctx context.Context, cutoffMillis int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM request_logs WHERE ts < ?`, cutoffMillis)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func requestLogListWhere(filter appproxy.RequestLogListFilter) (string, []any) {
	var clauses []string
	var args []any
	if value := strings.TrimSpace(filter.AccessProfile); value != "" {
		clauses = append(clauses, "(access_profile_id = ? OR access_profile = ?)")
		args = append(args, value, value)
	}
	if value := strings.TrimSpace(filter.Credential); value != "" {
		clauses = append(clauses, "(proxy_credential_id = ? OR proxy_credential = ?)")
		args = append(args, value, value)
	}
	if value := strings.TrimSpace(filter.NodeID); value != "" {
		clauses = append(clauses, "proxy_path_json LIKE ?")
		args = append(args, "%"+value+"%")
	}
	if value := strings.TrimSpace(filter.Target); value != "" {
		clauses = append(clauses, "target_host LIKE ?")
		args = append(args, "%"+value+"%")
	}
	switch strings.ToLower(strings.TrimSpace(filter.State)) {
	case appproxy.RequestLogStateRunning:
		clauses = append(clauses, "state = ?")
		args = append(args, appproxy.RequestLogStateRunning)
	case appproxy.RequestLogStateCompleted:
		clauses = append(clauses, "state = ?")
		args = append(args, appproxy.RequestLogStateCompleted)
	}
	switch strings.ToLower(strings.TrimSpace(filter.Result)) {
	case "true", "1", appproxy.RequestLogResultSuccess, "succeeded":
		clauses = append(clauses, "state = ? AND success = 1")
		args = append(args, appproxy.RequestLogStateCompleted)
	case "false", "0", appproxy.RequestLogResultFailure, "failed":
		clauses = append(clauses, "state = ? AND success = 0")
		args = append(args, appproxy.RequestLogStateCompleted)
	case appproxy.RequestLogResultRunning:
		clauses = append(clauses, "state = ?")
		args = append(args, appproxy.RequestLogStateRunning)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanRequestLog(row requestLogScanner) (appproxy.RequestLogEntry, error) {
	var item appproxy.RequestLogEntry
	var success sql.NullInt64
	err := row.Scan(
		&item.ID,
		&item.Timestamp,
		&item.ProxyCredentialID,
		&item.ProxyCredential,
		&item.AccessProfileID,
		&item.AccessProfile,
		&item.AccessProfileIdentifier,
		&item.TargetHost,
		&item.ProxyPath,
		&item.ProxyPathJSON,
		&item.State,
		&success,
		&item.FailureStage,
		&item.Error,
		&item.DurationMS,
		&item.IngressBytes,
		&item.EgressBytes,
		&item.HTTPStatus,
	)
	if err != nil {
		return appproxy.RequestLogEntry{}, err
	}
	if success.Valid {
		value := success.Int64 == 1
		item.Success = &value
	}
	return item, nil
}
