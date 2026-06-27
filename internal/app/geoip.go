package app

import (
	"context"

	maintenanceapp "proxygateway/internal/application/maintenance"
	geoipinfra "proxygateway/internal/infrastructure/geoip"
)

func (g *Gateway) geoIPStatus() map[string]any {
	status, _ := g.geoIPStatusRepo.LoadStatus(context.Background())
	var nextUpdateAt any
	if settings, err := g.loadMaintenanceSettings(); err == nil {
		if next, ok := maintenanceapp.NextGeoIPScheduledMillis(unixMillisNow(), settings.GeoIPUpdateTime); ok {
			nextUpdateAt = next
		}
	}
	return map[string]any{
		"file_path":      status.FilePath,
		"source":         geoipinfra.Source,
		"loaded_at":      status.LoadedAt,
		"updated_at":     status.UpdatedAt,
		"next_update_at": nextUpdateAt,
		"last_error":     status.LastError,
		"sha256":         status.SHA256,
		"loaded":         status.LoadedAt > 0,
	}
}
