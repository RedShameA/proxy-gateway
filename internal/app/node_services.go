package app

import applicationnodes "proxygateway/internal/application/nodes"

func (g *Gateway) nodeManagementService() applicationnodes.ManagementService {
	return applicationnodes.ManagementService{
		Repo:   g.nodeRepo,
		Runner: g.txRunners,
		NewNodeID: func() (string, error) {
			return prefixedID("node")
		},
		Now: unixMillisNow,
	}
}
