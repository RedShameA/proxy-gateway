package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

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
		listen:        net.Listen,
		notifyContext: productionNotifyContext,
	})
}

type gatewayRunner interface {
	Close() error
	Serve(net.Listener) error
}

type runDeps struct {
	dataDir       string
	listenAddr    string
	newLogger     func() (*zap.Logger, error)
	lookupEnv     func(string) (string, bool)
	newGateway    func(dataDir string, logger *zap.Logger, storageConfig storageinfra.Config) (gatewayRunner, error)
	listen        func(network, address string) (net.Listener, error)
	notifyContext func(context.Context) (context.Context, context.CancelFunc)
}

func productionNotifyContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
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
	shutdownCtx, stop := deps.notifyContext(context.Background())
	defer stop()
	var shutdownRequested atomic.Bool
	serveDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-serveDone:
			return
		case <-shutdownCtx.Done():
			select {
			case <-serveDone:
				return
			default:
			}
			shutdownRequested.Store(true)
			logger.Info("shutdown requested; closing listener")
			if closeErr := ln.Close(); closeErr != nil {
				logger.Warn("close listener for shutdown failed", zap.Error(closeErr))
			}
		}
	}()
	logger.Info("proxy gateway listening", zap.String("addr", ln.Addr().String()))
	err = gateway.Serve(ln)
	close(serveDone)
	<-watcherDone
	if err == nil {
		return 0
	}
	if shutdownRequested.Load() && errors.Is(err, net.ErrClosed) {
		logger.Info("proxy gateway shutdown complete")
		return 0
	}
	if err != nil {
		logger.Error("serve failed", zap.Error(err))
		return 1
	}
	return 0
}
