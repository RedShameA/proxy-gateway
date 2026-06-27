package app

import (
	"context"
	"net"
	"net/http"
	"os"

	appadmin "proxygateway/internal/application/admin"
	appproxy "proxygateway/internal/application/proxy"
	geoipinfra "proxygateway/internal/infrastructure/geoip"
	singboxinfra "proxygateway/internal/infrastructure/singbox"
	storageinfra "proxygateway/internal/infrastructure/storage"
	subscriptionfetch "proxygateway/internal/infrastructure/subscriptionfetch"
	"proxygateway/internal/interfaces/entrypoint"

	"go.uber.org/zap"
)

type Option func(*options)

type options struct {
	logger                   *zap.Logger
	disableMaintenanceRunner bool
}

func WithLogger(logger *zap.Logger) Option {
	return func(options *options) {
		options.logger = logger
	}
}

func WithoutMaintenanceRunner() Option {
	return func(options *options) {
		options.disableMaintenanceRunner = true
	}
}

func New(dataDir string, opts ...Option) (*Gateway, error) {
	var config options
	for _, opt := range opts {
		if opt != nil {
			opt(&config)
		}
	}
	logger := ensureLogger(config.logger)

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		logger.Error("create data directory failed", zap.String("data_dir", dataDir), zap.Error(err))
		return nil, err
	}
	store, err := storageinfra.Open(storageinfra.Config{DataDir: dataDir})
	if err != nil {
		logger.Error("open storage failed", zap.String("data_dir", dataDir), zap.Error(err))
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	protocolEngine, err := singboxinfra.NewNodeProtocolEngine()
	if err != nil {
		cancel()
		_ = store.Close()
		logger.Error("create protocol engine failed", zap.Error(err))
		return nil, err
	}
	g := &Gateway{
		store:               store,
		txRunners:           storageinfra.NewTxRunners(store),
		dataDir:             dataDir,
		protocolEngine:      protocolEngine,
		adminLogins:         appadmin.NewLoginLimiter(),
		ctx:                 ctx,
		cancel:              cancel,
		logger:              logger.Named("gateway"),
		subscriptionFetcher: subscriptionfetch.Fetch,
	}
	if err := g.migrate(); err != nil {
		_ = protocolEngine.Close()
		cancel()
		_ = store.Close()
		logger.Error("database migration failed", zap.String("data_dir", dataDir), zap.Error(err))
		return nil, err
	}
	repos, err := storageinfra.NewRepositories(store)
	if err != nil {
		_ = protocolEngine.Close()
		cancel()
		_ = store.Close()
		logger.Error("create storage repositories failed", zap.String("data_dir", dataDir), zap.Error(err))
		return nil, err
	}
	g.geoIPStatusRepo = repos.GeoIPStatus
	g.kvSettingsRepo = repos.KVSettings
	g.systemSettingsRepo = repos.SystemSettings
	g.adminRepo = repos.Admin
	g.maintenanceAuxRepo = repos.MaintenanceAux
	g.maintenanceRunRepo = repos.MaintenanceRun
	g.overviewRepo = repos.Overview
	g.dictionaryRepo = repos.Dictionary
	g.nodeRepo = repos.Node
	g.nodeObservationRepo = repos.NodeObservation
	g.evaluationRepo = repos.Evaluation
	g.requestLogRepo = repos.RequestLog
	g.proxyCredentialRepo = repos.ProxyCredential
	g.profileConfigRepo = repos.ProfileConfig
	g.profileCredentialRepo = repos.ProfileCredential
	g.subscriptionRepo = repos.Subscription
	if err := g.cancelExpiredMaintenanceRunsOnStartup(); err != nil {
		_ = protocolEngine.Close()
		cancel()
		_ = store.Close()
		logger.Error("startup cleanup failed", zap.String("data_dir", dataDir), zap.Error(err))
		return nil, err
	}
	g.geoIP = geoipinfra.NewService(dataDir, g.geoIPStatusRepo)
	g.geoIP.LoadExisting()
	if !config.disableMaintenanceRunner {
		g.maintenance = newMaintenanceRunner(g)
	}
	g.requestLogs = appproxy.NewRequestLogWriter(g.requestLogRepo, g.logger.Named("request_log_writer"))
	g.requestLogService = appproxy.NewRequestLogService(g.requestLogs, func() (string, error) {
		return prefixedID("log")
	}, g.logger.Named("request_log"))
	g.log().Info("gateway initialized", zap.String("data_dir", dataDir))
	return g, nil
}

func (g *Gateway) Close() error {
	if g == nil {
		return nil
	}
	var err error
	g.closeOnce.Do(func() {
		if g.cancel != nil {
			g.cancel()
		}
		if g.geoIP != nil {
			g.geoIP.Close()
		}
		if engine, ok := g.protocolEngine.(closeableNodeProtocolEngine); ok {
			if closeErr := engine.Close(); closeErr != nil && err == nil {
				err = closeErr
				g.log().Error("close protocol engine failed", zap.Error(closeErr))
			}
		}
		if g.requestLogs != nil {
			if ok := g.requestLogs.Close(appproxy.RequestLogFlushTimeout); !ok {
				g.log().Warn("request log writer close timed out", zap.Duration("timeout", appproxy.RequestLogFlushTimeout))
			}
		}
		if closeErr := g.store.Close(); closeErr != nil && err == nil {
			err = closeErr
			g.log().Error("close database failed", zap.Error(closeErr))
		}
	})
	return err
}

func (g *Gateway) Serve(ln net.Listener) error {
	if g.maintenance != nil {
		g.maintenance.start()
	}
	handler := entrypoint.Handler{
		HTTP:   g.Handler(),
		SOCKS5: g.handleSOCKS5Conn,
	}
	g.log().Info("gateway accepting connections", zap.String("addr", ln.Addr().String()))
	for {
		conn, err := ln.Accept()
		if err != nil {
			g.log().Error("listener accept failed", zap.Error(err))
			return err
		}
		go handler.ServeConn(conn)
	}
}

func (g *Gateway) Handler() http.Handler {
	api := g.httpAPIHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect || r.URL.IsAbs() {
			g.handleHTTPProxy(w, r)
			return
		}
		api.ServeHTTP(w, r)
	})
}
