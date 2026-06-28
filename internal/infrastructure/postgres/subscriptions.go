package postgres

import (
	"context"
	"database/sql"

	appsubscriptions "proxygateway/internal/application/subscriptions"
)

type SubscriptionRepository struct {
	db *sql.DB
}

func NewSubscriptionRepository(db *sql.DB) SubscriptionRepository {
	return SubscriptionRepository{db: db}
}

func (r SubscriptionRepository) List(ctx context.Context, filter appsubscriptions.ListFilter) (appsubscriptions.ListResult, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions`).Scan(&total); err != nil {
		return appsubscriptions.ListResult{}, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, name, source_type, url, content, imported_nodes, skipped_entries, skipped_summary_json,
		        last_error, auto_refresh_enabled, auto_refresh_interval_seconds, updated_at
		   FROM subscriptions
		  ORDER BY created_at, id
		  LIMIT $1 OFFSET $2`,
		limit,
		offset,
	)
	if err != nil {
		return appsubscriptions.ListResult{}, err
	}
	defer rows.Close()
	items := []appsubscriptions.Record{}
	for rows.Next() {
		record, err := scanSubscriptionRecord(rows)
		if err != nil {
			return appsubscriptions.ListResult{}, err
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return appsubscriptions.ListResult{}, err
	}
	return appsubscriptions.ListResult{Items: items, Total: total}, nil
}

func (r SubscriptionRepository) Load(ctx context.Context, id string) (appsubscriptions.Record, bool, error) {
	record, err := scanSubscriptionRecord(r.db.QueryRowContext(
		ctx,
		`SELECT id, name, source_type, url, content, imported_nodes, skipped_entries, skipped_summary_json,
		        last_error, auto_refresh_enabled, auto_refresh_interval_seconds, updated_at
		   FROM subscriptions
		  WHERE id = $1`,
		id,
	))
	if err == sql.ErrNoRows {
		return appsubscriptions.Record{}, false, nil
	}
	if err != nil {
		return appsubscriptions.Record{}, false, err
	}
	return record, true, nil
}

func (r SubscriptionRepository) LoadImportResult(ctx context.Context, id string) (appsubscriptions.ImportResultRecord, bool, error) {
	var record appsubscriptions.ImportResultRecord
	err := r.db.QueryRowContext(
		ctx,
		`SELECT id, imported_nodes, skipped_entries, skipped_summary_json
		   FROM subscriptions
		  WHERE id = $1`,
		id,
	).Scan(&record.ID, &record.ImportedNodes, &record.SkippedEntries, &record.SkippedSummaryJSON)
	if err == sql.ErrNoRows {
		return appsubscriptions.ImportResultRecord{}, false, nil
	}
	if err != nil {
		return appsubscriptions.ImportResultRecord{}, false, err
	}
	return record, true, nil
}

func (r SubscriptionRepository) UpdateAutoRefresh(ctx context.Context, id string, enabled *bool, intervalSeconds *int, updatedAt int64) (bool, error) {
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT 1 FROM subscriptions WHERE id = $1`, id).Scan(&exists); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if enabled != nil {
		if _, err := r.db.ExecContext(ctx, `UPDATE subscriptions SET auto_refresh_enabled = $1, updated_at = $2 WHERE id = $3`, *enabled, updatedAt, id); err != nil {
			return false, err
		}
	}
	if intervalSeconds != nil {
		if _, err := r.db.ExecContext(ctx, `UPDATE subscriptions SET auto_refresh_interval_seconds = $1, updated_at = $2 WHERE id = $3`, *intervalSeconds, updatedAt, id); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r SubscriptionRepository) StoreRefreshError(ctx context.Context, id, errorText string, updatedAt int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE subscriptions SET last_error = $1, updated_at = $2 WHERE id = $3`, errorText, updatedAt, id)
	return err
}

func scanSubscriptionRecord(row subscriptionRecordScanner) (appsubscriptions.Record, error) {
	var record appsubscriptions.Record
	var intervalSeconds int64
	err := row.Scan(
		&record.ID,
		&record.Name,
		&record.SourceType,
		&record.URL,
		&record.Content,
		&record.ImportedNodes,
		&record.SkippedEntries,
		&record.SkippedSummaryJSON,
		&record.LastError,
		&record.AutoRefreshEnabled,
		&intervalSeconds,
		&record.UpdatedAt,
	)
	record.AutoRefreshIntervalSeconds = int(intervalSeconds)
	return record, err
}

type subscriptionRecordScanner interface {
	Scan(dest ...any) error
}

var _ appsubscriptions.Repository = SubscriptionRepository{}
