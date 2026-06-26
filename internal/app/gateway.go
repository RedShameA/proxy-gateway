package app

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	geoipinfra "proxygateway/internal/infrastructure/geoip"
	storageinfra "proxygateway/internal/infrastructure/storage"
)

const (
	entrypointSniffTimeout = 5 * time.Second
	httpReadHeaderTimeout  = 10 * time.Second
)

func New(dataDir string) (*Gateway, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	store, err := storageinfra.Open(storageinfra.Config{DataDir: dataDir})
	if err != nil {
		return nil, err
	}
	db := store.DB
	ctx, cancel := context.WithCancel(context.Background())
	protocolEngine, err := newSingBoxNodeProtocolEngine()
	if err != nil {
		cancel()
		_ = db.Close()
		return nil, err
	}
	g := &Gateway{
		db:             db,
		dbDialect:      store.Dialect,
		dataDir:        dataDir,
		protocolEngine: protocolEngine,
		adminLogins:    newAdminLoginLimiter(),
		ctx:            ctx,
		cancel:         cancel,
	}
	if err := g.migrate(); err != nil {
		_ = protocolEngine.Close()
		cancel()
		_ = db.Close()
		return nil, err
	}
	if err := g.cancelExpiredMaintenanceRunsOnStartup(); err != nil {
		cancel()
		_ = db.Close()
		return nil, err
	}
	g.geoIP = geoipinfra.NewService(dataDir, db)
	g.geoIP.LoadExisting()
	g.maintenance = newMaintenanceRunner(g)
	g.requestLogs = newRequestLogWriter(db)
	return g, nil
}

func NewForTest(t testing.TB) *Gateway {
	t.Helper()
	g, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
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
			}
		}
		if g.requestLogs != nil {
			g.requestLogs.close(requestLogFlushTimeout)
		}
		if g.db != nil {
			if closeErr := g.db.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
	})
	return err
}

func (g *Gateway) Serve(ln net.Listener) error {
	if g.maintenance != nil {
		g.maintenance.start()
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go g.serveEntrypointConn(conn)
	}
}

func (g *Gateway) serveEntrypointConn(conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(entrypointSniffTimeout))
	reader := bufio.NewReader(conn)
	first, err := reader.Peek(1)
	if err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	buffered := &bufferedConn{Conn: conn, reader: reader}
	if first[0] == 0x05 {
		g.handleSOCKS5Conn(buffered)
		return
	}
	server := &http.Server{
		Handler:           g.Handler(),
		ReadHeaderTimeout: httpReadHeaderTimeout,
	}
	_ = server.Serve(&singleConnListener{conn: buffered})
}

func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/system/setup-status", g.handleSetupStatus)
	mux.HandleFunc("/api/admin/setup", g.handleAdminSetup)
	mux.HandleFunc("/api/admin/login", g.handleAdminLogin)
	mux.HandleFunc("/api/admin/me", g.handleAdminMe)
	mux.HandleFunc("/api/subscriptions", g.handleSubscriptions)
	mux.HandleFunc("/api/subscriptions/", g.handleSubscriptionSubroutes)
	mux.HandleFunc("/api/nodes", g.handleNodes)
	mux.HandleFunc("/api/nodes/", g.handleNodeSubroutes)
	mux.HandleFunc("/api/nodes/observations/run", g.handleRunNodeObservations)
	mux.HandleFunc("/api/access-profiles", g.handleAccessProfiles)
	mux.HandleFunc("/api/access-profiles/", g.handleAccessProfileSubroutes)
	mux.HandleFunc("/api/evaluation-settings", g.handleEvaluationSettings)
	mux.HandleFunc("/api/maintenance/runs", g.handleMaintenanceRuns)
	mux.HandleFunc("/api/maintenance/runs/", g.handleMaintenanceRunDetail)
	mux.HandleFunc("/api/maintenance/settings", g.handleMaintenanceSettings)
	mux.HandleFunc("/api/geoip", g.handleGeoIPStatus)
	mux.HandleFunc("/api/evaluations/run", g.handleRunEvaluations)
	mux.HandleFunc("/api/request-logs", g.handleRequestLogs)
	mux.HandleFunc("/api/overview", g.handleOverview)
	mux.HandleFunc("/api/dictionaries/egress-countries", g.handleEgressCountries)
	mux.HandleFunc("/api/system/settings", g.handleSystemSettings)
	mux.HandleFunc("/api/admin/password", g.handleAdminPassword)
	mux.HandleFunc("/", g.handleWebUI)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect || r.URL.IsAbs() {
			g.handleHTTPProxy(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
