package app

import geoipinfra "proxygateway/internal/infrastructure/geoip"

func (g *Gateway) geoIPStatus() map[string]any {
	var filePath, lastError, sha string
	var loadedAt, updatedAt int64
	_ = g.db.QueryRow(
		`SELECT file_path, loaded_at, updated_at, last_error, sha256 FROM geoip_status WHERE id = 1`,
	).Scan(&filePath, &loadedAt, &updatedAt, &lastError, &sha)
	var nextUpdateAt any
	if settings, err := g.loadMaintenanceSettings(); err == nil {
		if next, ok := nextGeoIPScheduledMillis(unixMillisNow(), settings.GeoIPUpdateTime); ok {
			nextUpdateAt = next
		}
	}
	return map[string]any{
		"file_path":      filePath,
		"source":         geoipinfra.Source,
		"loaded_at":      loadedAt,
		"updated_at":     updatedAt,
		"next_update_at": nextUpdateAt,
		"last_error":     lastError,
		"sha256":         sha,
		"loaded":         loadedAt > 0,
	}
}
