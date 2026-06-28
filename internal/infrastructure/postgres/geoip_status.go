package postgres

import (
	"context"
	"database/sql"

	appgeoip "proxygateway/internal/application/geoip"
)

type GeoIPStatusRepository struct {
	db *sql.DB
}

func NewGeoIPStatusRepository(db *sql.DB) GeoIPStatusRepository {
	return GeoIPStatusRepository{db: db}
}

func (r GeoIPStatusRepository) LoadStatus(ctx context.Context) (appgeoip.Status, error) {
	var status appgeoip.Status
	err := r.db.QueryRowContext(
		ctx,
		`SELECT file_path, loaded_at, updated_at, last_error, sha256 FROM geoip_status WHERE id = 1`,
	).Scan(&status.FilePath, &status.LoadedAt, &status.UpdatedAt, &status.LastError, &status.SHA256)
	if err != nil {
		return appgeoip.Status{}, err
	}
	return status, nil
}

func (r GeoIPStatusRepository) StoreStatus(ctx context.Context, update appgeoip.StatusUpdate) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO geoip_status (id, file_path, loaded_at, updated_at, last_error, sha256)
		 VALUES (1, $1, $2, $3, $4, $5)
		 ON CONFLICT(id) DO UPDATE SET
			file_path = CASE WHEN excluded.file_path != '' THEN excluded.file_path ELSE geoip_status.file_path END,
			loaded_at = CASE WHEN excluded.loaded_at != 0 THEN excluded.loaded_at ELSE geoip_status.loaded_at END,
			updated_at = CASE WHEN excluded.updated_at != 0 THEN excluded.updated_at ELSE geoip_status.updated_at END,
			last_error = excluded.last_error,
			sha256 = CASE WHEN excluded.sha256 != '' THEN excluded.sha256 ELSE geoip_status.sha256 END`,
		update.FilePath,
		update.LoadedAt,
		update.UpdatedAt,
		update.LastError,
		update.SHA256,
	)
	return err
}

var _ appgeoip.StatusRepository = GeoIPStatusRepository{}
