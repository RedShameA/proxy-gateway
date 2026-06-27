package app

import appadmin "proxygateway/internal/application/admin"

func (g *Gateway) adminService() appadmin.Service {
	return appadmin.Service{Repo: g.adminRepo, Now: unixMillisNow}
}
