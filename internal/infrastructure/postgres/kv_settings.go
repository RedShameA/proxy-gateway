package postgres

import (
	"context"
	"database/sql"

	appsettings "proxygateway/internal/application/settings"
)

type KVSettingsRepository struct {
	db *sql.DB
}

func NewKVSettingsRepository(db *sql.DB) KVSettingsRepository {
	return KVSettingsRepository{db: db}
}

func (r KVSettingsRepository) Get(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM kv_settings WHERE key = $1`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (r KVSettingsRepository) Set(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO kv_settings (key, value) VALUES ($1, $2)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key,
		value,
	)
	return err
}

var _ appsettings.KVRepository = KVSettingsRepository{}
