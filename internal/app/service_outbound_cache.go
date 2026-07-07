package app

import (
	"context"
	"errors"
	"strings"

	applicationnodes "proxygateway/internal/application/nodes"
	appprofiles "proxygateway/internal/application/profiles"
	appproxy "proxygateway/internal/application/proxy"
	domainprofile "proxygateway/internal/domain/profile"

	"go.uber.org/zap"
)

const serviceOutboundProfilePageSize = 500

type serviceOutboundBuildPlan struct {
	Allowed []appproxy.ServiceOutboundPath
	Warm    []appproxy.ServiceOutboundPath
}

func (g *Gateway) triggerServiceOutboundSync(reason string) uint64 {
	controller, ok := g.serviceOutboundController()
	if !ok || g.ctx == nil {
		return 0
	}
	g.ensureServiceOutboundSyncLoop()
	generation := g.serviceOutboundSyncGeneration.Add(1)
	controller.SetServiceOutboundSyncGeneration(generation)
	g.log().Debug("service outbound sync queued",
		zap.String("reason", reason),
		zap.Uint64("generation", generation),
	)
	select {
	case g.serviceOutboundSyncCh <- struct{}{}:
	default:
	}
	return generation
}

func (g *Gateway) ensureServiceOutboundSyncLoop() {
	g.serviceOutboundSyncOnce.Do(func() {
		g.serviceOutboundSyncCh = make(chan struct{}, 1)
		go g.serviceOutboundSyncLoop(g.serviceOutboundSyncCh)
	})
}

func (g *Gateway) serviceOutboundSyncLoop(signal <-chan struct{}) {
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-signal:
		}
		for {
			generation := g.serviceOutboundSyncGeneration.Load()
			plan, err := g.buildGlobalServiceOutboundPlan(g.ctx)
			if err != nil {
				g.log().Warn("build service outbound allowed set failed", zap.Uint64("generation", generation), zap.Error(err))
				break
			}
			if g.serviceOutboundSyncGeneration.Load() != generation {
				continue
			}
			controller, ok := g.serviceOutboundController()
			if !ok {
				break
			}
			fields := append([]zap.Field{zap.Uint64("generation", generation)}, serviceOutboundPlanLogFields(plan)...)
			if err := controller.SyncServiceOutboundCache(generation, plan.Allowed); err != nil {
				g.log().Warn("sync service outbound cache failed", append(fields, zap.Error(err))...)
			} else {
				g.log().Debug("service outbound cache sync applied", fields...)
			}
			if len(plan.Warm) > 0 && g.serviceOutboundSyncGeneration.Load() == generation {
				if err := controller.WarmServiceOutboundPaths(plan.Warm); err != nil {
					g.log().Warn("warm service outbound cache failed", append(fields, zap.Error(err))...)
				} else {
					g.log().Debug("service outbound cache warm requested", fields...)
				}
			}
			if g.serviceOutboundSyncGeneration.Load() == generation {
				break
			}
		}
	}
}

func (g *Gateway) serviceOutboundController() (serviceOutboundCacheController, bool) {
	if g == nil || g.protocolEngine == nil {
		return nil, false
	}
	controller, ok := g.protocolEngine.(serviceOutboundCacheController)
	return controller, ok
}

func (g *Gateway) buildGlobalServiceOutboundPlan(ctx context.Context) (serviceOutboundBuildPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	configs, err := g.loadAllAccessProfileConfigs(ctx)
	if err != nil {
		return serviceOutboundBuildPlan{}, err
	}
	limits := g.serviceOutboundCacheLimits()
	var plan serviceOutboundBuildPlan
	seenAllowed := map[string]struct{}{}
	seenWarm := map[string]struct{}{}
	for _, cfg := range configs {
		allowed, warm, err := g.serviceOutboundPathsForProfile(ctx, cfg, limits)
		if err != nil {
			return serviceOutboundBuildPlan{}, err
		}
		for _, path := range allowed {
			appendUniqueServiceOutboundPath(&plan.Allowed, seenAllowed, path)
		}
		for _, path := range warm {
			appendUniqueServiceOutboundPath(&plan.Warm, seenWarm, path)
		}
	}
	return plan, nil
}

func (g *Gateway) loadAllAccessProfileConfigs(ctx context.Context) ([]accessProfileConfig, error) {
	if g.profileConfigRepo == nil {
		return nil, nil
	}
	var configs []accessProfileConfig
	offset := 0
	for {
		result, err := g.profileConfigRepo.ListConfigIDs(ctx, appprofiles.ListConfigFilter{Limit: serviceOutboundProfilePageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, profileID := range result.IDs {
			cfg, found, err := g.profileConfigRepo.LoadConfig(ctx, profileID)
			if err != nil {
				return nil, err
			}
			if !found {
				continue
			}
			cfg.ApplyDefaults()
			configs = append(configs, cfg)
		}
		offset += len(result.IDs)
		if len(result.IDs) == 0 || offset >= result.Total {
			break
		}
	}
	return configs, nil
}

func (g *Gateway) serviceOutboundPathsForProfile(ctx context.Context, cfg accessProfileConfig, limits appproxy.ServiceOutboundCacheLimits) ([]appproxy.ServiceOutboundPath, []appproxy.ServiceOutboundPath, error) {
	if cfg.Type == domainprofile.TypeRandom {
		nodes, err := g.candidateNodesWithContext(ctx, cfg.CandidateFilter())
		if err != nil {
			return nil, nil, err
		}
		nodes = g.usableNodesWithContext(ctx, nodes)
		if limits.Single > 0 && len(nodes) > limits.Single {
			g.log().Warn("random profile service outbound candidates exceed hard cap",
				zap.String("profile_id", cfg.ID),
				zap.Int("candidate_count", len(nodes)),
				zap.Int("hard_cap", limits.Single),
			)
		}
		paths := make([]appproxy.ServiceOutboundPath, 0, len(nodes))
		for _, node := range nodes {
			paths = append(paths, appproxy.SingleServiceOutboundPath(node))
		}
		return paths, paths, nil
	}
	path, err := appproxy.SelectPathForCredential(proxyCredentialRecord{ProfileID: cfg.ID}, cfg, g.servicePathSelectionDeps(ctx))
	if err != nil {
		if servicePathSelectionMeansNoPath(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	servicePath, ok := selectedPathToServiceOutboundPath(path)
	if !ok {
		return nil, nil, nil
	}
	return []appproxy.ServiceOutboundPath{servicePath}, nil, nil
}

func (g *Gateway) servicePathSelectionDeps(ctx context.Context) appproxy.PathSelectionDeps {
	return appproxy.PathSelectionDeps{
		CandidateNodes: func(filter candidateFilter) ([]nodeRecord, error) {
			return g.candidateNodesWithContext(ctx, filter)
		},
		UsableNodes: func(nodes []nodeRecord) []nodeRecord {
			return g.usableNodesWithContext(ctx, nodes)
		},
		RandomIndex: func(int) (int, error) {
			return 0, nil
		},
		ChainPathMatchesProfile: func(cfg accessProfileConfig, frontNodeID, exitNodeID string) bool {
			return g.chainPathMatchesProfileWithContext(ctx, cfg, frontNodeID, exitNodeID)
		},
		LoadUsableNode: func(nodeID string) (nodeRecord, error) {
			return g.loadUsableNodeWithContext(ctx, nodeID)
		},
		ProfileNodeMatchesCandidateFilter: func(profileID, nodeID string, filter candidateFilter) bool {
			return g.profileNodeMatchesCandidateFilterWithContext(ctx, profileID, nodeID, filter)
		},
	}
}

func selectedPathToServiceOutboundPath(path selectedProxyPath) (appproxy.ServiceOutboundPath, bool) {
	if path.FrontNode.ID != "" && path.ExitNode.ID != "" {
		return appproxy.ChainServiceOutboundPath(path.FrontNode, path.ExitNode), true
	}
	if path.Node.ID != "" {
		return appproxy.SingleServiceOutboundPath(path.Node), true
	}
	return appproxy.ServiceOutboundPath{}, false
}

func servicePathSelectionMeansNoPath(err error) bool {
	return errors.Is(err, appproxy.ErrNoUsableProxyPath) ||
		errors.Is(err, applicationnodes.ErrNodeNotFound) ||
		strings.Contains(err.Error(), "node is disabled")
}

func (g *Gateway) warmCurrentServiceOutboundPath(ctx context.Context, profileID string) {
	controller, ok := g.serviceOutboundController()
	if !ok {
		return
	}
	cfg, err := g.loadAccessProfileConfig(profileID)
	if err != nil {
		return
	}
	paths, _, err := g.serviceOutboundPathsForProfile(ctx, cfg, controller.ServiceOutboundCacheLimits())
	if err != nil || len(paths) == 0 {
		return
	}
	g.warmServiceOutboundPaths(paths)
}

func (g *Gateway) warmServiceOutboundPaths(paths []appproxy.ServiceOutboundPath) {
	controller, ok := g.serviceOutboundController()
	if !ok || len(paths) == 0 {
		return
	}
	go func() {
		if err := controller.WarmServiceOutboundPaths(paths); err != nil {
			g.log().Warn("warm service outbound cache failed", zap.Error(err))
			return
		}
		single, chain := serviceOutboundPathCounts(paths)
		g.log().Debug("service outbound cache warm requested",
			zap.Int("warm_paths", len(paths)),
			zap.Int("warm_single_paths", single),
			zap.Int("warm_chain_paths", chain),
		)
	}()
}

func (g *Gateway) serviceOutboundCacheLimits() appproxy.ServiceOutboundCacheLimits {
	if controller, ok := g.serviceOutboundController(); ok {
		return controller.ServiceOutboundCacheLimits()
	}
	return appproxy.ServiceOutboundCacheLimits{}
}

func appendUniqueServiceOutboundPath(paths *[]appproxy.ServiceOutboundPath, seen map[string]struct{}, path appproxy.ServiceOutboundPath) {
	key := serviceOutboundPathLogicalKey(path)
	if key == "" {
		return
	}
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*paths = append(*paths, path)
}

func serviceOutboundPathLogicalKey(path appproxy.ServiceOutboundPath) string {
	if path.IsChain() {
		if path.FrontNode.ID == "" || path.ExitNode.ID == "" {
			return ""
		}
		return "chain:" + path.FrontNode.ID + "->" + path.ExitNode.ID
	}
	if path.Node.ID == "" {
		return ""
	}
	return "single:" + path.Node.ID
}

func serviceOutboundPlanLogFields(plan serviceOutboundBuildPlan) []zap.Field {
	allowedSingle, allowedChain := serviceOutboundPathCounts(plan.Allowed)
	warmSingle, warmChain := serviceOutboundPathCounts(plan.Warm)
	return []zap.Field{
		zap.Int("allowed_paths", len(plan.Allowed)),
		zap.Int("allowed_single_paths", allowedSingle),
		zap.Int("allowed_chain_paths", allowedChain),
		zap.Int("warm_paths", len(plan.Warm)),
		zap.Int("warm_single_paths", warmSingle),
		zap.Int("warm_chain_paths", warmChain),
	}
}

func serviceOutboundPathCounts(paths []appproxy.ServiceOutboundPath) (int, int) {
	var single int
	var chain int
	for _, path := range paths {
		if path.IsChain() {
			chain++
			continue
		}
		single++
	}
	return single, chain
}
