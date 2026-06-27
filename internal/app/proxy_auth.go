package app

import (
	"context"

	appproxy "proxygateway/internal/application/proxy"

	"go.uber.org/zap"
)

func (g *Gateway) loadProxyCredentialForProxy(username, password string) (proxyCredentialRecord, string, string, bool) {
	result := g.proxyAccessService().Authenticate(context.Background(), username, password)
	if result.LookupErr != nil {
		g.log().Warn("lookup proxy credential failed", zap.String("profile_identifier", username), zap.Error(result.LookupErr))
	}
	if result.TouchErr != nil {
		g.log().Warn("touch proxy credential last used failed",
			zap.String("credential_id", result.Credential.ID),
			zap.String("profile_id", result.Credential.ProfileID),
			zap.Error(result.TouchErr),
		)
	}
	if !result.OK {
		return proxyCredentialRecord{}, result.Failure.Stage, result.Failure.Error, false
	}
	return result.Credential, "", "", true
}

func (g *Gateway) proxyAccessService() appproxy.AccessService {
	return appproxy.AccessService{
		Credentials: g.proxyCredentialRepo,
		LoadProfileConfig: func(ctx context.Context, profileID string) (accessProfileConfig, error) {
			record, found, err := g.profileConfigRepo.LoadConfig(ctx, profileID)
			if err != nil {
				return accessProfileConfig{}, err
			}
			if !found {
				return accessProfileConfig{}, appproxy.ErrAccessProfileConfigNotFound
			}
			record.ApplyDefaults()
			return record, nil
		},
		Path: appproxy.PathSelectionDeps{
			CandidateNodes:                    g.candidateNodes,
			UsableNodes:                       g.usableNodes,
			RandomIndex:                       cryptoRandomIndex,
			ChainPathMatchesProfile:           g.chainPathMatchesProfile,
			LoadUsableNode:                    g.loadUsableNode,
			ProfileNodeMatchesCandidateFilter: g.profileNodeMatchesCandidateFilter,
		},
		NowMillis:         unixMillisNow,
		TouchWindowMillis: 60_000,
	}
}
