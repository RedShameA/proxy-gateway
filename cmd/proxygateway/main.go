package main

import (
	"net"
	"os"

	"proxygateway/internal/app"

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
	logger, err := app.NewProcessLoggerFromEnv()
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

	gateway, err := app.New(defaultDataDir, app.WithLogger(logger))
	if err != nil {
		logger.Error("open gateway failed", zap.String("data_dir", defaultDataDir), zap.Error(err))
		return 1
	}
	defer func() {
		if err := gateway.Close(); err != nil {
			logger.Error("close gateway failed", zap.Error(err))
		}
	}()

	ln, err := net.Listen("tcp", defaultListenAddr)
	if err != nil {
		logger.Error("listen failed", zap.String("addr", defaultListenAddr), zap.Error(err))
		return 1
	}
	logger.Info("proxy gateway listening", zap.String("addr", ln.Addr().String()))
	if err := gateway.Serve(ln); err != nil {
		logger.Error("serve failed", zap.Error(err))
		return 1
	}
	return 0
}
