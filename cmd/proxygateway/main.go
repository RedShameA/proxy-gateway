package main

import (
	"log"
	"net"

	"proxygateway/internal/app"
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
	app.SetBuildInfo(app.BuildInfo{
		Version:  version,
		Revision: revision,
		Source:   source,
		License:  "AGPL-3.0-or-later",
	})

	gateway, err := app.New(defaultDataDir)
	if err != nil {
		log.Fatalf("open gateway: %v", err)
	}
	defer gateway.Close()

	ln, err := net.Listen("tcp", defaultListenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", defaultListenAddr, err)
	}
	log.Printf("proxy gateway listening on %s", ln.Addr())
	if err := gateway.Serve(ln); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
