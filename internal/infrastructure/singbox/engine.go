package singbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/adapter/inbound"
	sbOutbound "github.com/sagernet/sing-box/adapter/outbound"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/dns"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	S "github.com/sagernet/sing/common"
	sJson "github.com/sagernet/sing/common/json"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/service"

	appproxy "proxygateway/internal/application/proxy"
	appsubscriptions "proxygateway/internal/application/subscriptions"

	"go.uber.org/zap"
)

type singBoxNodeProtocolEngine struct {
	builder                     *singBoxOutboundBuilder
	cache                       *singBoxOutboundCache
	temporaryMu                 sync.Mutex
	temporaryCaches             map[*singBoxOutboundCache]struct{}
	closing                     bool
	closeOnce                   sync.Once
	closeErr                    error
	latestServiceSyncGeneration atomic.Uint64
	logger                      *zap.Logger
}

func NewNodeProtocolEngine(loggers ...*zap.Logger) (*singBoxNodeProtocolEngine, error) {
	builder, err := newSingBoxOutboundBuilder()
	if err != nil {
		return nil, err
	}
	logger := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &singBoxNodeProtocolEngine{
		builder:         builder,
		cache:           newSingBoxServiceOutboundCache(),
		temporaryCaches: make(map[*singBoxOutboundCache]struct{}),
		logger:          logger,
	}, nil
}

func (e *singBoxNodeProtocolEngine) DialNode(node appproxy.Node, target string, timeouts appproxy.DialTimeouts) (appproxy.DialResult, error) {
	return e.dialNode(e.cache, node, target, timeouts)
}

func (e *singBoxNodeProtocolEngine) dialNode(cache *singBoxOutboundCache, node appproxy.Node, target string, timeouts appproxy.DialTimeouts) (appproxy.DialResult, error) {
	if !node.Enabled {
		return appproxy.DialResult{}, errors.New("node disabled")
	}
	targetAddr, err := socksaddrFromTarget(target)
	if err != nil {
		return appproxy.DialResult{}, err
	}
	entry, metrics, err := e.acquireSingleOutbound(cache, node)
	if err != nil {
		return appproxy.DialResult{Metrics: metrics}, err
	}
	return dialCachedOutbound(cache, entry, targetAddr, timeouts, metrics)
}

func (e *singBoxNodeProtocolEngine) acquireSingleOutbound(cache *singBoxOutboundCache, node appproxy.Node) (*singBoxCachedOutbound, appproxy.DialMetrics, error) {
	if cache == nil {
		return nil, appproxy.DialMetrics{}, errSingBoxOutboundCacheClosed
	}
	if !node.Enabled {
		return nil, appproxy.DialMetrics{}, errors.New("node disabled")
	}
	outboundJSON, fingerprint, err := runtimeOutboundJSONForNode(node)
	if err != nil {
		return nil, appproxy.DialMetrics{}, err
	}
	return cache.acquireSingle(fingerprint, func() (*singBoxCachedOutbound, error) {
		outbound, err := e.builder.buildSingle(outboundJSON, "node-"+shortRuntimeTag(fingerprint))
		if err != nil {
			return nil, err
		}
		return newSingBoxCachedOutbound(fingerprint, []string{fingerprint}, outbound, func() error {
			return closeSingBoxOutbound(outbound)
		}), nil
	})
}

func (e *singBoxNodeProtocolEngine) DialChain(frontNode, exitNode appproxy.Node, target string, timeouts appproxy.DialTimeouts) (appproxy.DialResult, error) {
	return e.dialChain(e.cache, frontNode, exitNode, target, timeouts)
}

func (e *singBoxNodeProtocolEngine) dialChain(cache *singBoxOutboundCache, frontNode, exitNode appproxy.Node, target string, timeouts appproxy.DialTimeouts) (appproxy.DialResult, error) {
	if !frontNode.Enabled || !exitNode.Enabled {
		return appproxy.DialResult{}, errors.New("node disabled")
	}
	targetAddr, err := socksaddrFromTarget(target)
	if err != nil {
		return appproxy.DialResult{}, err
	}
	entry, metrics, err := e.acquireChainOutbound(cache, frontNode, exitNode)
	if err != nil {
		return appproxy.DialResult{Metrics: metrics}, err
	}
	return dialCachedOutbound(cache, entry, targetAddr, timeouts, metrics)
}

func (e *singBoxNodeProtocolEngine) acquireChainOutbound(cache *singBoxOutboundCache, frontNode, exitNode appproxy.Node) (*singBoxCachedOutbound, appproxy.DialMetrics, error) {
	if cache == nil {
		return nil, appproxy.DialMetrics{}, errSingBoxOutboundCacheClosed
	}
	if !frontNode.Enabled || !exitNode.Enabled {
		return nil, appproxy.DialMetrics{}, errors.New("node disabled")
	}
	if appsubscriptions.NormalizeNodeType(exitNode.Type) == "direct" {
		return e.acquireSingleOutbound(cache, frontNode)
	}
	frontJSON, frontFingerprint, err := runtimeOutboundJSONForNode(frontNode)
	if err != nil {
		return nil, appproxy.DialMetrics{}, err
	}
	exitJSON, exitFingerprint, err := runtimeOutboundJSONForNode(exitNode)
	if err != nil {
		return nil, appproxy.DialMetrics{}, err
	}
	chainKey := frontFingerprint + "->" + exitFingerprint
	return cache.acquireChain(chainKey, []string{frontFingerprint, exitFingerprint}, func() (*singBoxCachedOutbound, error) {
		frontTag := "chain-front-" + shortRuntimeTag(frontFingerprint)
		exitTag := "chain-exit-" + shortRuntimeTag(exitFingerprint)
		graph, err := e.builder.buildChain(frontJSON, exitJSON, frontTag, exitTag)
		if err != nil {
			return nil, err
		}
		return newSingBoxCachedOutbound(chainKey, []string{frontFingerprint, exitFingerprint}, graph.exitOutbound, graph.Close), nil
	})
}

type singBoxOutboundDialOutcome struct {
	conn      net.Conn
	err       error
	elapsedMS int64
}

func (e *singBoxNodeProtocolEngine) dialCachedOutbound(entry *singBoxCachedOutbound, targetAddr M.Socksaddr, timeouts appproxy.DialTimeouts, metrics appproxy.DialMetrics) (appproxy.DialResult, error) {
	return dialCachedOutbound(e.cache, entry, targetAddr, timeouts, metrics)
}

func dialCachedOutbound(cache *singBoxOutboundCache, entry *singBoxCachedOutbound, targetAddr M.Socksaddr, timeouts appproxy.DialTimeouts, metrics appproxy.DialMetrics) (appproxy.DialResult, error) {
	ctx, cancel := dialContextForTimeouts(timeouts)
	dialStart := time.Now()
	done := make(chan singBoxOutboundDialOutcome, 1)
	go func() {
		conn, err := entry.outbound.DialContext(ctx, "tcp", targetAddr)
		done <- singBoxOutboundDialOutcome{
			conn:      conn,
			err:       err,
			elapsedMS: durationMillis(dialStart),
		}
	}()

	select {
	case outcome := <-done:
		cancel()
		cache.release(entry)
		metrics.OutboundDialMS = outcome.elapsedMS
		return appproxy.DialResult{Conn: outcome.conn, Metrics: metrics}, outcome.err
	case <-ctx.Done():
		timeoutErr := fmt.Errorf("sing-box outbound dial timeout: %w", ctx.Err())
		cancel()
		metrics.OutboundDialMS = durationMillis(dialStart)
		go func() {
			outcome := <-done
			if outcome.conn != nil {
				_ = outcome.conn.Close()
			}
			cache.release(entry)
		}()
		return appproxy.DialResult{Metrics: metrics}, timeoutErr
	}
}

func (e *singBoxNodeProtocolEngine) InvalidateFingerprint(fingerprint string) {
	if e == nil || e.cache == nil {
		return
	}
	temporaryCaches := e.activeTemporaryCaches()
	e.cache.invalidateFingerprint(fingerprint)
	for _, cache := range temporaryCaches {
		cache.invalidateFingerprint(fingerprint)
	}
	e.log().Debug("outbound cache fingerprint invalidated",
		append([]zap.Field{
			zap.String("fingerprint", fingerprint),
			zap.Int("affected_temporary_caches", len(temporaryCaches)),
		}, serviceOutboundCacheStatsFields(e.ServiceOutboundCacheStats())...)...,
	)
}

func (e *singBoxNodeProtocolEngine) Close() error {
	if e == nil {
		return nil
	}
	e.closeOnce.Do(func() {
		var err error
		serviceStats := e.cache.stats()
		temporaryCaches := e.closeTemporaryRegistry()
		var temporaryStats singBoxOutboundCacheStats
		for _, cache := range temporaryCaches {
			stats := cache.stats()
			temporaryStats.single += stats.single
			temporaryStats.chain += stats.chain
			temporaryStats.building += stats.building
			err = errors.Join(err, cache.closeAll())
		}
		if e.cache != nil {
			err = errors.Join(err, e.cache.closeAll())
		}
		if e.builder != nil {
			err = errors.Join(err, e.builder.Close())
		}
		e.closeErr = err
		e.log().Debug("node protocol engine closing caches",
			zap.Int("service_single", serviceStats.single),
			zap.Int("service_chain", serviceStats.chain),
			zap.Int("service_building", serviceStats.building),
			zap.Int("closed_temporary_caches", len(temporaryCaches)),
			zap.Int("temporary_single", temporaryStats.single),
			zap.Int("temporary_chain", temporaryStats.chain),
			zap.Int("temporary_building", temporaryStats.building),
		)
	})
	return e.closeErr
}

func (e *singBoxNodeProtocolEngine) NewTemporaryNodeProtocolEngine() (appproxy.TemporaryNodeProtocolEngine, error) {
	if e == nil {
		return nil, errSingBoxOutboundCacheClosed
	}
	cache := newSingBoxTemporaryOutboundCache()
	e.temporaryMu.Lock()
	if e.closing {
		e.temporaryMu.Unlock()
		_ = cache.closeAll()
		return nil, errSingBoxOutboundCacheClosed
	}
	if e.temporaryCaches == nil {
		e.temporaryCaches = make(map[*singBoxOutboundCache]struct{})
	}
	e.temporaryCaches[cache] = struct{}{}
	e.temporaryMu.Unlock()
	e.log().Debug("temporary outbound cache registered",
		serviceOutboundCacheStatsFields(e.ServiceOutboundCacheStats())...,
	)
	return &singBoxTemporaryNodeProtocolEngine{parent: e, cache: cache}, nil
}

func (e *singBoxNodeProtocolEngine) activeTemporaryCaches() []*singBoxOutboundCache {
	e.temporaryMu.Lock()
	defer e.temporaryMu.Unlock()
	caches := make([]*singBoxOutboundCache, 0, len(e.temporaryCaches))
	for cache := range e.temporaryCaches {
		caches = append(caches, cache)
	}
	return caches
}

func (e *singBoxNodeProtocolEngine) closeTemporaryRegistry() []*singBoxOutboundCache {
	e.temporaryMu.Lock()
	defer e.temporaryMu.Unlock()
	e.closing = true
	caches := make([]*singBoxOutboundCache, 0, len(e.temporaryCaches))
	for cache := range e.temporaryCaches {
		caches = append(caches, cache)
	}
	e.temporaryCaches = nil
	return caches
}

func (e *singBoxNodeProtocolEngine) unregisterTemporaryCache(cache *singBoxOutboundCache) int {
	if e == nil || cache == nil {
		return 0
	}
	e.temporaryMu.Lock()
	defer e.temporaryMu.Unlock()
	delete(e.temporaryCaches, cache)
	return len(e.temporaryCaches)
}

type singBoxTemporaryNodeProtocolEngine struct {
	parent    *singBoxNodeProtocolEngine
	cache     *singBoxOutboundCache
	closeErr  error
	closeOnce sync.Once
}

func (e *singBoxTemporaryNodeProtocolEngine) DialNode(node appproxy.Node, target string, timeouts appproxy.DialTimeouts) (appproxy.DialResult, error) {
	if e == nil || e.parent == nil {
		return appproxy.DialResult{}, errSingBoxOutboundCacheClosed
	}
	return e.parent.dialNode(e.cache, node, target, timeouts)
}

func (e *singBoxTemporaryNodeProtocolEngine) DialChain(frontNode, exitNode appproxy.Node, target string, timeouts appproxy.DialTimeouts) (appproxy.DialResult, error) {
	if e == nil || e.parent == nil {
		return appproxy.DialResult{}, errSingBoxOutboundCacheClosed
	}
	return e.parent.dialChain(e.cache, frontNode, exitNode, target, timeouts)
}

func (e *singBoxTemporaryNodeProtocolEngine) Close() error {
	if e == nil {
		return nil
	}
	e.closeOnce.Do(func() {
		stats := e.cache.stats()
		if e.cache != nil {
			e.closeErr = e.cache.closeAll()
		}
		activeTemporaryCaches := 0
		if e.parent != nil {
			activeTemporaryCaches = e.parent.unregisterTemporaryCache(e.cache)
			e.parent.log().Debug("temporary outbound cache closed",
				zap.Int("temporary_single", stats.single),
				zap.Int("temporary_chain", stats.chain),
				zap.Int("temporary_building", stats.building),
				zap.Int("active_temporary_caches", activeTemporaryCaches),
			)
		}
	})
	return e.closeErr
}

func (e *singBoxNodeProtocolEngine) SetServiceOutboundSyncGeneration(generation uint64) {
	if e == nil {
		return
	}
	atomicMaxUint64(&e.latestServiceSyncGeneration, generation)
}

func (e *singBoxNodeProtocolEngine) SyncServiceOutboundCache(generation uint64, paths []appproxy.ServiceOutboundPath) error {
	if e == nil || e.cache == nil {
		return errSingBoxOutboundCacheClosed
	}
	if generation < e.latestServiceSyncGeneration.Load() {
		e.log().Debug("service outbound cache sync skipped stale generation",
			zap.Uint64("generation", generation),
			zap.Uint64("latest_generation", e.latestServiceSyncGeneration.Load()),
		)
		return nil
	}
	e.SetServiceOutboundSyncGeneration(generation)
	allowed, normalizeErr := serviceOutboundAllowedKeys(paths)
	if generation < e.latestServiceSyncGeneration.Load() {
		e.log().Debug("service outbound cache sync skipped stale generation",
			zap.Uint64("generation", generation),
			zap.Uint64("latest_generation", e.latestServiceSyncGeneration.Load()),
		)
		return nil
	}
	syncErr := e.cache.syncServiceAllowedKeys(allowed)
	e.log().Debug("service outbound cache synced",
		append([]zap.Field{
			zap.Uint64("generation", generation),
			zap.Int("allowed_single", len(allowed.single)),
			zap.Int("allowed_chain", len(allowed.chain)),
		}, serviceOutboundCacheStatsFields(e.ServiceOutboundCacheStats())...)...,
	)
	return errors.Join(normalizeErr, syncErr)
}

func (e *singBoxNodeProtocolEngine) WarmServiceOutboundPaths(paths []appproxy.ServiceOutboundPath) error {
	if e == nil || e.cache == nil || len(paths) == 0 {
		return nil
	}
	const warmConcurrency = 4
	workers := warmConcurrency
	if len(paths) < workers {
		workers = len(paths)
	}
	jobs := make(chan appproxy.ServiceOutboundPath)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var warmErr error
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				if err := e.warmServiceOutboundPath(path); err != nil {
					errMu.Lock()
					warmErr = errors.Join(warmErr, err)
					errMu.Unlock()
				}
			}
		}()
	}
	for _, path := range paths {
		jobs <- path
	}
	close(jobs)
	wg.Wait()
	e.log().Debug("service outbound cache warm completed",
		append([]zap.Field{
			zap.Int("requested_paths", len(paths)),
		}, serviceOutboundCacheStatsFields(e.ServiceOutboundCacheStats())...)...,
	)
	return warmErr
}

func (e *singBoxNodeProtocolEngine) ServiceOutboundCacheStats() appproxy.ServiceOutboundCacheStats {
	if e == nil {
		return appproxy.ServiceOutboundCacheStats{}
	}
	serviceStats := e.cache.stats()
	temporaryCaches := e.activeTemporaryCaches()
	var temporaryStats singBoxOutboundCacheStats
	for _, cache := range temporaryCaches {
		stats := cache.stats()
		temporaryStats.single += stats.single
		temporaryStats.chain += stats.chain
		temporaryStats.building += stats.building
	}
	return appproxy.ServiceOutboundCacheStats{
		ServiceSingle:         serviceStats.single,
		ServiceChain:          serviceStats.chain,
		ServiceBuilding:       serviceStats.building,
		ActiveTemporaryCaches: len(temporaryCaches),
		TemporarySingle:       temporaryStats.single,
		TemporaryChain:        temporaryStats.chain,
		TemporaryBuilding:     temporaryStats.building,
	}
}

func serviceOutboundCacheStatsFields(stats appproxy.ServiceOutboundCacheStats) []zap.Field {
	return []zap.Field{
		zap.Int("service_single", stats.ServiceSingle),
		zap.Int("service_chain", stats.ServiceChain),
		zap.Int("service_building", stats.ServiceBuilding),
		zap.Int("active_temporary_caches", stats.ActiveTemporaryCaches),
		zap.Int("temporary_single", stats.TemporarySingle),
		zap.Int("temporary_chain", stats.TemporaryChain),
		zap.Int("temporary_building", stats.TemporaryBuilding),
	}
}

func (e *singBoxNodeProtocolEngine) ServiceOutboundCacheLimits() appproxy.ServiceOutboundCacheLimits {
	return appproxy.ServiceOutboundCacheLimits{
		Single: serviceSingleOutboundHardCap,
		Chain:  serviceChainOutboundHardCap,
	}
}

func (e *singBoxNodeProtocolEngine) warmServiceOutboundPath(path appproxy.ServiceOutboundPath) error {
	var entry *singBoxCachedOutbound
	var err error
	if path.IsChain() {
		entry, _, err = e.acquireChainOutbound(e.cache, path.FrontNode, path.ExitNode)
	} else {
		entry, _, err = e.acquireSingleOutbound(e.cache, path.Node)
	}
	if err != nil {
		return err
	}
	e.cache.release(entry)
	return nil
}

func (e *singBoxNodeProtocolEngine) log() *zap.Logger {
	if e == nil || e.logger == nil {
		return zap.NewNop()
	}
	return e.logger
}

func serviceOutboundAllowedKeys(paths []appproxy.ServiceOutboundPath) (singBoxOutboundAllowedKeys, error) {
	allowed := singBoxOutboundAllowedKeys{
		single: make(map[string]struct{}),
		chain:  make(map[string]struct{}),
	}
	var err error
	for _, path := range paths {
		key, keyErr := serviceOutboundCacheKey(path)
		if keyErr != nil {
			err = errors.Join(err, keyErr)
			continue
		}
		switch key.namespace {
		case singBoxOutboundCacheSingle:
			allowed.single[key.key] = struct{}{}
		case singBoxOutboundCacheChain:
			allowed.chain[key.key] = struct{}{}
		}
	}
	return allowed, err
}

type singBoxServiceOutboundCacheKey struct {
	namespace singBoxOutboundCacheNamespace
	key       string
}

func serviceOutboundCacheKey(path appproxy.ServiceOutboundPath) (singBoxServiceOutboundCacheKey, error) {
	if path.IsChain() {
		if path.FrontNode.ID == "" || path.ExitNode.ID == "" {
			return singBoxServiceOutboundCacheKey{}, errors.New("chain service outbound path requires front and exit nodes")
		}
		_, frontFingerprint, err := runtimeOutboundJSONForNode(path.FrontNode)
		if err != nil {
			return singBoxServiceOutboundCacheKey{}, err
		}
		if appsubscriptions.NormalizeNodeType(path.ExitNode.Type) == "direct" {
			return singBoxServiceOutboundCacheKey{namespace: singBoxOutboundCacheSingle, key: frontFingerprint}, nil
		}
		_, exitFingerprint, err := runtimeOutboundJSONForNode(path.ExitNode)
		if err != nil {
			return singBoxServiceOutboundCacheKey{}, err
		}
		return singBoxServiceOutboundCacheKey{namespace: singBoxOutboundCacheChain, key: frontFingerprint + "->" + exitFingerprint}, nil
	}
	if path.Node.ID == "" {
		return singBoxServiceOutboundCacheKey{}, errors.New("single service outbound path requires node")
	}
	_, fingerprint, err := runtimeOutboundJSONForNode(path.Node)
	if err != nil {
		return singBoxServiceOutboundCacheKey{}, err
	}
	return singBoxServiceOutboundCacheKey{namespace: singBoxOutboundCacheSingle, key: fingerprint}, nil
}

func atomicMaxUint64(value *atomic.Uint64, next uint64) {
	for {
		current := value.Load()
		if next <= current {
			return
		}
		if value.CompareAndSwap(current, next) {
			return
		}
	}
}

type singBoxOutboundBuilder struct {
	ctx                 context.Context
	registry            *sbOutbound.Registry
	endpointRegistry    adapter.EndpointRegistry
	endpointManager     adapter.EndpointManager
	logFactory          log.Factory
	dnsTransportManager *dns.TransportManager
	dnsRouter           *dns.Router
	tagSeq              atomic.Uint64
}

func newSingBoxOutboundBuilder() (*singBoxOutboundBuilder, error) {
	ctx := include.Context(context.Background())
	logFactory := log.NewNOPFactory()
	logger := logFactory.NewLogger("proxy-gateway")

	endpointRegistry := service.FromContext[adapter.EndpointRegistry](ctx)
	endpointManager := endpoint.NewManager(logger, endpointRegistry)
	service.MustRegister[adapter.EndpointManager](ctx, endpointManager)

	inboundManager := inbound.NewManager(logger, service.FromContext[adapter.InboundRegistry](ctx), endpointManager)
	service.MustRegister[adapter.InboundManager](ctx, inboundManager)

	baseOutboundManager := sbOutbound.NewManager(logger, service.FromContext[adapter.OutboundRegistry](ctx), endpointManager, "")
	service.MustRegister[adapter.OutboundManager](ctx, baseOutboundManager)

	dnsTransportManager := dns.NewTransportManager(logger, service.FromContext[adapter.DNSTransportRegistry](ctx), baseOutboundManager, "local")
	service.MustRegister[adapter.DNSTransportManager](ctx, dnsTransportManager)
	if err := dnsTransportManager.Create(ctx, logger, "local", C.DNSTypeLocal, &option.LocalDNSServerOptions{}); err != nil {
		return nil, fmt.Errorf("create local dns transport: %w", err)
	}
	if err := dnsTransportManager.Start(adapter.StartStateInitialize); err != nil {
		return nil, fmt.Errorf("initialize dns transport manager: %w", err)
	}
	if err := dnsTransportManager.Start(adapter.StartStateStart); err != nil {
		_ = dnsTransportManager.Close()
		return nil, fmt.Errorf("start dns transport manager: %w", err)
	}

	dnsRouter := dns.NewRouter(ctx, logFactory, option.DNSOptions{})
	service.MustRegister[adapter.DNSRouter](ctx, dnsRouter)
	if err := dnsRouter.Initialize(nil); err != nil {
		_ = dnsTransportManager.Close()
		return nil, fmt.Errorf("initialize dns router: %w", err)
	}
	if err := dnsRouter.Start(adapter.StartStateStart); err != nil {
		_ = dnsRouter.Close()
		_ = dnsTransportManager.Close()
		return nil, fmt.Errorf("start dns router: %w", err)
	}

	registry, ok := service.FromContext[adapter.OutboundRegistry](ctx).(*sbOutbound.Registry)
	if !ok {
		_ = dnsRouter.Close()
		_ = dnsTransportManager.Close()
		return nil, fmt.Errorf("unexpected outbound registry type %T", service.FromContext[adapter.OutboundRegistry](ctx))
	}
	return &singBoxOutboundBuilder{
		ctx:                 ctx,
		registry:            registry,
		endpointRegistry:    endpointRegistry,
		endpointManager:     endpointManager,
		logFactory:          logFactory,
		dnsTransportManager: dnsTransportManager,
		dnsRouter:           dnsRouter,
	}, nil
}

func (b *singBoxOutboundBuilder) buildSingle(rawOutboundJSON, tag string) (adapter.Outbound, error) {
	raw, err := runtimeOutboundJSON(rawOutboundJSON, tag, "")
	if err != nil {
		return nil, err
	}
	config, err := b.parseOutbound(b.ctx, raw)
	if err != nil {
		return nil, err
	}
	outbound, err := b.registry.CreateOutbound(
		b.ctx,
		nil,
		b.logFactory.NewLogger("outbound/"+config.Type),
		config.Tag,
		config.Type,
		config.Options,
	)
	if err != nil {
		return nil, fmt.Errorf("create outbound [%s]: %w", config.Type, err)
	}
	for _, stage := range adapter.ListStartStages {
		if err := adapter.LegacyStart(outbound, stage); err != nil {
			_ = closeSingBoxOutbound(outbound)
			return nil, fmt.Errorf("outbound start %s [%s]: %w", stage, config.Type, err)
		}
	}
	return outbound, nil
}

func (b *singBoxOutboundBuilder) buildChain(frontJSON, exitJSON, frontTag, exitTag string) (*singBoxChainGraph, error) {
	graphBuilder, err := newSingBoxOutboundBuilder()
	if err != nil {
		return nil, err
	}
	ctx := graphBuilder.ctx
	manager := sbOutbound.NewManager(graphBuilder.logFactory.NewLogger("chain/outbound"), graphBuilder.registry, graphBuilder.endpointManager, "")
	service.MustRegister[adapter.OutboundManager](ctx, manager)

	frontRaw, err := runtimeOutboundJSON(frontJSON, frontTag, "")
	if err != nil {
		_ = graphBuilder.Close()
		return nil, err
	}
	exitRaw, err := runtimeOutboundJSON(exitJSON, exitTag, frontTag)
	if err != nil {
		_ = graphBuilder.Close()
		return nil, err
	}
	frontConfig, err := graphBuilder.parseOutbound(ctx, frontRaw)
	if err != nil {
		_ = graphBuilder.Close()
		return nil, err
	}
	exitConfig, err := graphBuilder.parseOutbound(ctx, exitRaw)
	if err != nil {
		_ = graphBuilder.Close()
		return nil, err
	}
	if err := manager.Create(ctx, nil, graphBuilder.logFactory.NewLogger("chain/"+frontConfig.Type), frontConfig.Tag, frontConfig.Type, frontConfig.Options); err != nil {
		_ = graphBuilder.Close()
		return nil, fmt.Errorf("create chain front outbound [%s]: %w", frontConfig.Type, err)
	}
	if err := manager.Create(ctx, nil, graphBuilder.logFactory.NewLogger("chain/"+exitConfig.Type), exitConfig.Tag, exitConfig.Type, exitConfig.Options); err != nil {
		_ = manager.Close()
		_ = graphBuilder.Close()
		return nil, fmt.Errorf("create chain exit outbound [%s]: %w", exitConfig.Type, err)
	}
	manager.Initialize(func() (adapter.Outbound, error) {
		return graphBuilder.createDirectFallback(ctx)
	})
	for _, stage := range adapter.ListStartStages {
		if err := manager.Start(stage); err != nil {
			_ = manager.Close()
			_ = graphBuilder.Close()
			return nil, fmt.Errorf("start chain outbound graph %s: %w", stage, err)
		}
	}
	exitOutbound, ok := manager.Outbound(exitTag)
	if !ok {
		_ = manager.Close()
		_ = graphBuilder.Close()
		return nil, errors.New("chain exit outbound not found after build")
	}
	return &singBoxChainGraph{builder: graphBuilder, manager: manager, exitOutbound: exitOutbound}, nil
}

func (b *singBoxOutboundBuilder) parseOutbound(ctx context.Context, raw json.RawMessage) (option.Outbound, error) {
	var config option.Outbound
	if err := sJson.UnmarshalContext(ctx, raw, &config); err != nil {
		return option.Outbound{}, fmt.Errorf("parse outbound options: %w", err)
	}
	if strings.TrimSpace(config.Type) == "" {
		return option.Outbound{}, errors.New("outbound type is required")
	}
	if strings.TrimSpace(config.Tag) == "" {
		return option.Outbound{}, errors.New("outbound tag is required")
	}
	return config, nil
}

func (b *singBoxOutboundBuilder) createDirectFallback(ctx context.Context) (adapter.Outbound, error) {
	tag := fmt.Sprintf("direct-fallback-%d", b.tagSeq.Add(1))
	raw, err := runtimeOutboundJSON(`{"type":"direct"}`, tag, "")
	if err != nil {
		return nil, err
	}
	config, err := b.parseOutbound(ctx, raw)
	if err != nil {
		return nil, err
	}
	return b.registry.CreateOutbound(ctx, nil, b.logFactory.NewLogger("chain/direct"), config.Tag, config.Type, config.Options)
}

func runtimeOutboundJSONForNode(node appproxy.Node) (string, string, error) {
	outboundJSON := strings.TrimSpace(node.OutboundJSON)
	if outboundJSON == "" {
		var err error
		outboundJSON, err = appsubscriptions.NormalizeNodeOutboundJSON(appsubscriptions.ParsedNode{
			Type:       node.Type,
			Server:     node.Server,
			ServerPort: node.ServerPort,
			Username:   node.Username,
			Password:   node.Password,
		})
		if err != nil {
			return "", "", err
		}
	}
	return outboundJSON, appsubscriptions.OutboundFingerprint(outboundJSON), nil
}

func ValidateOutboundJSON(outboundJSON string) error {
	builder, err := newSingBoxOutboundBuilder()
	if err != nil {
		return err
	}
	defer builder.Close()
	raw, err := runtimeOutboundJSON(outboundJSON, "test-outbound", "")
	if err != nil {
		return err
	}
	_, err = builder.parseOutbound(builder.ctx, raw)
	return err
}

func (b *singBoxOutboundBuilder) Close() error {
	return errors.Join(
		func() error {
			if b.dnsRouter != nil {
				return b.dnsRouter.Close()
			}
			return nil
		}(),
		func() error {
			if b.dnsTransportManager != nil {
				return b.dnsTransportManager.Close()
			}
			return nil
		}(),
	)
}

type singBoxChainGraph struct {
	builder      *singBoxOutboundBuilder
	manager      *sbOutbound.Manager
	exitOutbound adapter.Outbound
}

func (g *singBoxChainGraph) Close() error {
	if g == nil {
		return nil
	}
	var err error
	if g.manager != nil {
		err = errors.Join(err, g.manager.Close())
	}
	if g.builder != nil {
		err = errors.Join(err, g.builder.Close())
	}
	return err
}

func runtimeOutboundJSON(rawOutboundJSON, tag, detour string) (json.RawMessage, error) {
	var outbound map[string]any
	if err := json.Unmarshal([]byte(rawOutboundJSON), &outbound); err != nil {
		return nil, err
	}
	outbound["tag"] = tag
	if detour != "" {
		outbound["detour"] = detour
	}
	raw, err := json.Marshal(outbound)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func dialContextForTimeouts(timeouts appproxy.DialTimeouts) (context.Context, context.CancelFunc) {
	ctx := context.Background()
	deadline := timeouts.Deadline
	if timeouts.ConnectTimeout > 0 {
		connectDeadline := time.Now().Add(timeouts.ConnectTimeout)
		if deadline.IsZero() || connectDeadline.Before(deadline) {
			deadline = connectDeadline
		}
	}
	if !deadline.IsZero() {
		ctx, cancel := context.WithDeadline(ctx, deadline)
		return ctx, cancel
	}
	return ctx, func() {}
}

func socksaddrFromTarget(target string) (M.Socksaddr, error) {
	host, portText, err := net.SplitHostPort(target)
	if err != nil {
		return M.Socksaddr{}, err
	}
	port, err := parsePort(portText)
	if err != nil {
		return M.Socksaddr{}, err
	}
	return M.ParseSocksaddrHostPort(host, uint16(port)), nil
}

func parsePort(portText string) (int, error) {
	var port int
	for _, ch := range portText {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid target port %q", portText)
		}
		port = port*10 + int(ch-'0')
	}
	if port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid target port %q", portText)
	}
	return port, nil
}

func closeSingBoxOutbound(outbound adapter.Outbound) error {
	if outbound == nil {
		return nil
	}
	return S.Close(outbound)
}

func shortRuntimeTag(fingerprint string) string {
	if len(fingerprint) <= 16 {
		return fingerprint
	}
	return fingerprint[:16]
}
