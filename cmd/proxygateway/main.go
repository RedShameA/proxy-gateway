package main

import (
	"net"
	"os"

	"proxygateway/internal/app"
	storageinfra "proxygateway/internal/infrastructure/storage"

	"go.uber.org/zap"
)

const (
	defaultDataDir    = "/data"
	defaultListenAddr = ":8080"
	defaultSourceURL  = "https://github.com/RedShameA/proxy-gateway"
)

var (
	version  = "dev"
	revision = "unknown"
	source   = defaultSourceURL
)

func main() {
	os.Exit(run())
}

func run() int {
	return runWithDeps(runDeps{
		dataDir:    defaultDataDir,
		listenAddr: defaultListenAddr,
		newLogger:  app.NewProcessLoggerFromEnv,
		lookupEnv:  os.LookupEnv,
		newGateway: func(dataDir string, logger *zap.Logger, storageConfig storageinfra.Config) (gatewayRunner, error) {
			return app.New(dataDir, app.WithLogger(logger), app.WithStorageConfig(storageConfig))
		},
		listen: net.Listen,
	})
}

type gatewayRunner interface {
	Close() error
	Serve(net.Listener) error
}

type runDeps struct {
	dataDir    string
	listenAddr string
	newLogger  func() (*zap.Logger, error)
	lookupEnv  func(string) (string, bool)
	newGateway func(dataDir string, logger *zap.Logger, storageConfig storageinfra.Config) (gatewayRunner, error)
	listen     func(network, address string) (net.Listener, error)
}

func runWithDeps(deps runDeps) int {
	logger, err := deps.newLogger()
	if err != nil {
		panic(err)
	}
	defer func() { _ = logger.Sync() }()

	app.SetBuildInfo(app.BuildInfo{
		Version:  version,
		Revision: revision,
		Source:   source,
		License:  "AGPL-3.0-or-later",
	})

	storageConfig, err := storageinfra.ConfigFromEnv(deps.dataDir, deps.lookupEnv)
	if err != nil {
		logger.Error("database configuration failed", zap.Error(err))
		return 1
	}

	gateway, err := deps.newGateway(deps.dataDir, logger, storageConfig)
	if err != nil {
		logger.Error("open gateway failed", zap.String("data_dir", deps.dataDir), zap.String("db_driver", storageConfig.Driver), zap.Error(err))
		return 1
	}
	defer func() {
		if err := gateway.Close(); err != nil {
			logger.Error("close gateway failed", zap.Error(err))
		}
	}()

	ln, err := deps.listen("tcp", deps.listenAddr)
	if err != nil {
		logger.Error("listen failed", zap.String("addr", deps.listenAddr), zap.Error(err))
		return 1
	}
	logger.Info("proxy gateway listening", zap.String("addr", ln.Addr().String()))
	if err := gateway.Serve(ln); err != nil {
		logger.Error("serve failed", zap.Error(err))
		return 1
	}
	return 0
}
