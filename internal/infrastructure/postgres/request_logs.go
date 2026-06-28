package postgres

import (
	"context"
	"database/sql"
	"strings"

	appproxy "proxygateway/internal/application/proxy"
)

type RequestLogRepository struct {
	db *sql.DB
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
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NULL, '', '', 0, 0, 0, 0)`,
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
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE request_logs
		    SET state = $1,
		        success = $2,
		        failure_stage = $3,
		        error = $4,
		        duration_ms = $5,
		        ingress_bytes = $6,
		        egress_bytes = $7,
		        http_status = $8
		  WHERE id = $9`,
		appproxy.RequestLogStateCompleted,
		record.Success,
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
		 ) VALUES ($1, $2, '', '', '', $3, $4, $5, '', '', $6, false, $7, $8, $9, 0, 0, $10)`,
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
		  ORDER BY ts DESC, sequence DESC, id DESC
		  LIMIT $`+placeholder(len(queryArgs)-1)+` OFFSET $`+placeholder(len(queryArgs)),
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
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_logs WHERE ts > $1`, cutoffMillis).Scan(&counts.Total); err != nil {
		return appproxy.RequestLogCounts{}, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_logs WHERE ts > $1 AND state = $2 AND success = false`, cutoffMillis, appproxy.RequestLogStateCompleted).Scan(&counts.Failed); err != nil {
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
		  WHERE state = $1 AND success = false
		  ORDER BY ts DESC, sequence DESC, id DESC
		  LIMIT $2`,
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
	res, err := r.db.ExecContext(ctx, `DELETE FROM request_logs WHERE ts < $1`, cutoffMillis)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func requestLogListWhere(filter appproxy.RequestLogListFilter) (string, []any) {
	clauses := []string{}
	args := []any{}
	addClause := func(clause string, values ...any) {
		for _, value := range values {
			args = append(args, value)
			clause = strings.Replace(clause, "?", "$"+placeholder(len(args)), 1)
		}
		clauses = append(clauses, clause)
	}
	if value := strings.TrimSpace(filter.AccessProfile); value != "" {
		addClause("(access_profile_id = ? OR access_profile = ?)", value, value)
	}
	if value := strings.TrimSpace(filter.Credential); value != "" {
		addClause("(proxy_credential_id = ? OR proxy_credential = ?)", value, value)
	}
	if value := strings.TrimSpace(filter.NodeID); value != "" {
		addClause("proxy_path_json LIKE ?", "%"+value+"%")
	}
	if value := strings.TrimSpace(filter.Target); value != "" {
		addClause("target_host LIKE ?", "%"+value+"%")
	}
	switch strings.ToLower(strings.TrimSpace(filter.State)) {
	case appproxy.RequestLogStateRunning:
		addClause("state = ?", appproxy.RequestLogStateRunning)
	case appproxy.RequestLogStateCompleted:
		addClause("state = ?", appproxy.RequestLogStateCompleted)
	}
	switch strings.ToLower(strings.TrimSpace(filter.Result)) {
	case "true", "1", appproxy.RequestLogResultSuccess, "succeeded":
		addClause("state = ? AND success = true", appproxy.RequestLogStateCompleted)
	case "false", "0", appproxy.RequestLogResultFailure, "failed":
		addClause("state = ? AND success = false", appproxy.RequestLogStateCompleted)
	case appproxy.RequestLogResultRunning:
		addClause("state = ?", appproxy.RequestLogStateRunning)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

type requestLogScanner interface {
	Scan(dest ...any) error
}

func scanRequestLog(row requestLogScanner) (appproxy.RequestLogEntry, error) {
	var item appproxy.RequestLogEntry
	var success sql.NullBool
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
		value := success.Bool
		item.Success = &value
	}
	return item, nil
}

var _ appproxy.RequestLogRepository = RequestLogRepository{}
