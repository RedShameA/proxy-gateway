package app

import (
	"context"

	maintenanceapp "proxygateway/internal/application/maintenance"
	applicationprofiles "proxygateway/internal/application/profiles"
)

func (g *Gateway) profileManagementService() applicationprofiles.ManagementService {
	return applicationprofiles.ManagementService{
		Config:      g.profileConfigService(),
		Credentials: g.profileCredentialService(),
		Summary:     g.profileSummaryService(),
		Detail:      g.profileDetailService(),
		Delete: applicationprofiles.DeleteService{
			Runner: g.txRunners,
		},
	}
}

func (g *Gateway) profileConfigService() applicationprofiles.ConfigService {
	return applicationprofiles.NewConfigService(applicationprofiles.ConfigServiceDeps{
		Repository: g.profileConfigRepo,
		NewID: func() (string, error) {
			return prefixedID("profile")
		},
		Now:               unixMillisNow,
		Validation:        g.accessProfileConfigValidationDeps(),
		UpdateWithRelease: applicationprofiles.UpdateConfigWithRelease(g.txRunners),
	})
}

func (g *Gateway) accessProfileConfigValidationDeps() applicationprofiles.ConfigValidationDeps {
	return applicationprofiles.ConfigValidationDeps{
		DefaultTestURL: defaultProfileTestURL,
		IdentifierExists: func(identifier, excludeProfileID string) (bool, error) {
			return g.profileConfigRepo.ProfileIdentifierExists(context.Background(), identifier, excludeProfileID)
		},
		NodeExists: func(nodeID string) (bool, error) {
			_, err := g.loadNode(nodeID)
			return err == nil, nil
		},
	}
}

func (g *Gateway) profileCredentialService() applicationprofiles.CredentialService {
	return applicationprofiles.NewCredentialService(g.profileCredentialRepo, func() (string, error) {
		return prefixedID("cred")
	}, unixMillisNow)
}

func (g *Gateway) profileSummaryService() applicationprofiles.SummaryService {
	return applicationprofiles.NewSummaryService(applicationprofiles.SummaryServiceDeps{
		Configs:     g.profileConfigRepo,
		Credentials: g.profileCredentialRepo,
		CurrentPath: g.accessProfileCurrentPath,
	})
}

func (g *Gateway) profileDetailService() applicationprofiles.DetailService {
	return applicationprofiles.NewDetailService(applicationprofiles.DetailServiceDeps{
		Configs:                      g.profileConfigRepo,
		Credentials:                  g.profileCredentialRepo,
		NodeUsable:                   g.nodeUsable,
		UnknownCountryCandidateCount: g.unknownCountryCandidateCount,
		CurrentPath:                  g.accessProfileCurrentPath,
		CandidateNodeIDs: func(filter candidateFilter) ([]string, error) {
			nodes, err := g.candidateNodes(filter)
			if err != nil {
				return nil, err
			}
			nodeIDs := make([]string, 0, len(nodes))
			for _, node := range nodes {
				nodeIDs = append(nodeIDs, node.ID)
			}
			return nodeIDs, nil
		},
		RecentEvents: func(ctx context.Context, profileID string, limit int) ([]map[string]any, error) {
			events, err := g.maintenanceRunService().ListProfileEvents(ctx, profileID, limit)
			if err != nil {
				return nil, err
			}
			recentEvents := make([]map[string]any, 0, len(events))
			for _, event := range events {
				recentEvents = append(recentEvents, maintenanceapp.RunToMap(event))
			}
			return recentEvents, nil
		},
	})
}

func (g *Gateway) accessProfileCurrentPath(cfg accessProfileConfig) any {
	return applicationprofiles.BuildCurrentPath(cfg, applicationprofiles.CurrentPathDeps{
		ChainPathMatchesProfile:           g.chainPathMatchesProfile,
		ProfileNodeMatchesCandidateFilter: g.profileNodeMatchesCandidateFilter,
		NodePathSummary:                   g.nodePathSummary,
	})
}

func (g *Gateway) nodePathSummary(nodeID string) (applicationprofiles.NodePathSummary, bool) {
	node, err := g.loadNode(nodeID)
	if err != nil {
		return applicationprofiles.NodePathSummary{}, false
	}
	return applicationprofiles.BuildNodePathSummary(nodeRecordToApplication(node), g.nodeObservationSnapshot(nodeID)), true
}
