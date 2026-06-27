package app

import (
	maintenanceapp "proxygateway/internal/application/maintenance"
)

func (g *Gateway) maintenanceRunService() *maintenanceapp.Service {
	return maintenanceapp.NewService(
		g.maintenanceRunRepo,
		prefixedID,
		unixMillisNow,
	)
}
