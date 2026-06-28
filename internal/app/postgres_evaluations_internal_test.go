package app

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	maintenanceapp "proxygateway/internal/application/maintenance"
	appnodes "proxygateway/internal/application/nodes"
	postgresinfra "proxygateway/internal/infrastructure/postgres"
	storageinfra "proxygateway/internal/infrastructure/storage"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestPostgresProfileEvaluationRunSucceedsWithPartialCandidateFailuresAndDebugLogs(t *testing.T) {
	t.Parallel()

	g, observed := newObservedPostgresGatewayForInternalTest(t)
	g.protocolEngine = postgresEvaluationTestEngine{failingNodeID: "node_pg_bad"}
	insertPostgresEvaluationTestNode(t, g, "node_pg_ok", "pg-ok", 100)
	insertPostgresEvaluationTestNode(t, g, "node_pg_bad", "pg-bad", 101)
	cfg := defaultAccessProfileConfig("profile_pg_partial_candidates")
	cfg.Name = "pg partial candidates"
	cfg.Type = "fastest"
	cfg.TestURL = "http://example.test/probe"
	cfg.MinEvaluationIntervalSeconds = 0
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	runID, err := g.enqueueProfileEvaluationRun(cfg.ID, cfg.Name, maintenanceapp.TriggerManual, cfg.ConfigVersion, true)
	if err != nil {
		t.Fatal(err)
	}

	if err := g.runMaintenanceRun(runID); err != nil {
		t.Fatal(err)
	}

	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != maintenanceRunStateFinished || run.Result != maintenanceRunResultSuccess || run.ReasonCode != "initial_selection" {
		t.Fatalf("profile evaluation run = %#v, want finished success initial_selection", run)
	}
	if run.TotalCount != 2 || run.FinishedCount != 2 {
		t.Fatalf("profile evaluation counts = %d/%d, want 2/2", run.FinishedCount, run.TotalCount)
	}
	detail := maintenanceRunDetail(run)
	if detail["candidate_count"] != float64(2) || detail["success_count"] != float64(1) || detail["failure_count"] != float64(1) || detail["selected_node_id"] != "node_pg_ok" {
		t.Fatalf("profile evaluation detail = %#v, want one success, one failure, selected node_pg_ok", detail)
	}

	candidateLogs := observed.FilterMessage("profile candidate probe result").All()
	if len(candidateLogs) != 2 {
		t.Fatalf("candidate debug logs = %d, want 2: %#v", len(candidateLogs), candidateLogs)
	}
	okLog := observedLogWithStringField(t, candidateLogs, "node_id", "node_pg_ok")
	okFields := okLog.ContextMap()
	assertStringField(t, okFields, "profile_id", cfg.ID)
	assertStringField(t, okFields, "node_name", "pg-ok")
	assertBoolField(t, okFields, "success", true)
	assertIntField(t, okFields, "http_status", http.StatusNoContent)
	assertStringField(t, okFields, "error", "")

	badLog := observedLogWithStringField(t, candidateLogs, "node_id", "node_pg_bad")
	badFields := badLog.ContextMap()
	assertStringField(t, badFields, "node_name", "pg-bad")
	assertBoolField(t, badFields, "success", false)
	assertStringField(t, badFields, "error", "candidate dial failed")
	assertFieldAbsent(t, badFields, "http_status")

	selectionLogs := observed.FilterMessage("profile evaluation selected path").All()
	if len(selectionLogs) != 1 {
		t.Fatalf("selection debug logs = %d, want 1: %#v", len(selectionLogs), selectionLogs)
	}
	selectionFields := selectionLogs[0].ContextMap()
	assertStringField(t, selectionFields, "profile_id", cfg.ID)
	assertStringField(t, selectionFields, "profile_type", "fastest")
	assertIntField(t, selectionFields, "candidate_count", 2)
	assertIntField(t, selectionFields, "failure_count", 1)
	assertStringField(t, selectionFields, "best_node_id", "node_pg_ok")
	assertStringField(t, selectionFields, "selected_node_id", "node_pg_ok")
	assertStringField(t, selectionFields, "switch_reason", "initial_selection")
}

func TestPostgresChainLinkEvaluationRunSelectsFrontExitPathAndDebugLogs(t *testing.T) {
	t.Parallel()

	g, observed := newObservedPostgresGatewayForInternalTest(t)
	g.protocolEngine = postgresEvaluationTestEngine{
		delays: map[string]time.Duration{
			"node_pg_slow_front": 80 * time.Millisecond,
			"node_pg_fast_front": 0,
		},
	}
	insertPostgresEvaluationTestNode(t, g, "node_pg_exit", "pg-exit", 100)
	insertPostgresEvaluationTestNode(t, g, "node_pg_slow_front", "pg-slow-front", 101)
	insertPostgresEvaluationTestNode(t, g, "node_pg_fast_front", "pg-fast-front", 102)
	cfg := defaultAccessProfileConfig("profile_pg_chain_link")
	cfg.Name = "pg chain link"
	cfg.Type = "chain"
	cfg.ExitNodeIDs = []string{"node_pg_exit"}
	cfg.ChainEvaluationMode = "chain_link"
	cfg.TestURL = "http://example.test/not-used"
	cfg.MinEvaluationIntervalSeconds = 0
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	runID, err := g.enqueueProfileEvaluationRun(cfg.ID, cfg.Name, maintenanceapp.TriggerManual, cfg.ConfigVersion, true)
	if err != nil {
		t.Fatal(err)
	}

	if err := g.runMaintenanceRun(runID); err != nil {
		t.Fatal(err)
	}

	run, err := g.loadMaintenanceRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.State != maintenanceRunStateFinished || run.Result != maintenanceRunResultSuccess || run.ReasonCode != "initial_selection" {
		t.Fatalf("chain evaluation run = %#v, want finished success initial_selection", run)
	}
	detail := maintenanceRunDetail(run)
	if detail["candidate_count"] != float64(2) || detail["success_count"] != float64(2) || detail["selected_node_id"] != "node_pg_fast_front" || detail["selected_exit_node_id"] != "node_pg_exit" {
		t.Fatalf("chain evaluation detail = %#v, want selected front/exit", detail)
	}

	candidateLogs := observed.FilterMessage("chain candidate probe result").All()
	if len(candidateLogs) != 2 {
		t.Fatalf("chain candidate debug logs = %d, want 2: %#v", len(candidateLogs), candidateLogs)
	}
	fastLog := observedLogWithStringField(t, candidateLogs, "front_node_id", "node_pg_fast_front")
	fastFields := fastLog.ContextMap()
	assertStringField(t, fastFields, "profile_id", cfg.ID)
	assertStringField(t, fastFields, "front_node_name", "pg-fast-front")
	assertStringField(t, fastFields, "exit_node_id", "node_pg_exit")
	assertStringField(t, fastFields, "exit_node_name", "pg-exit")
	assertBoolField(t, fastFields, "success", true)
	assertStringField(t, fastFields, "error", "")
	assertFieldAbsent(t, fastFields, "http_status")

	selectionLogs := observed.FilterMessage("profile evaluation selected path").All()
	if len(selectionLogs) != 1 {
		t.Fatalf("selection debug logs = %d, want 1: %#v", len(selectionLogs), selectionLogs)
	}
	selectionFields := selectionLogs[0].ContextMap()
	assertStringField(t, selectionFields, "profile_id", cfg.ID)
	assertStringField(t, selectionFields, "profile_type", "chain")
	assertIntField(t, selectionFields, "candidate_count", 2)
	assertIntField(t, selectionFields, "failure_count", 0)
	assertStringField(t, selectionFields, "best_node_id", "node_pg_fast_front")
	assertStringField(t, selectionFields, "best_exit_node_id", "node_pg_exit")
	assertStringField(t, selectionFields, "selected_node_id", "node_pg_fast_front")
	assertStringField(t, selectionFields, "selected_exit_node_id", "node_pg_exit")
	assertStringField(t, selectionFields, "switch_reason", "initial_selection")
}

func TestPostgresFastestEvaluationRunKeepsStableSwitchReasons(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		currentNodeID string
		bestNodeID    string
		delays        map[string]time.Duration
		wantReason    string
	}{
		{
			name:          "candidate clearly better",
			currentNodeID: "node_pg_current_slow",
			bestNodeID:    "node_pg_challenger_fast",
			delays: map[string]time.Duration{
				"node_pg_current_slow":    80 * time.Millisecond,
				"node_pg_challenger_fast": 0,
			},
			wantReason: "candidate_clearly_better",
		},
		{
			name:          "current path still best",
			currentNodeID: "node_pg_current_fast",
			bestNodeID:    "node_pg_current_fast",
			delays: map[string]time.Duration{
				"node_pg_current_fast":    0,
				"node_pg_challenger_slow": 80 * time.Millisecond,
			},
			wantReason: "current_path_still_best",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g, _ := newObservedPostgresGatewayForInternalTest(t)
			g.protocolEngine = postgresEvaluationTestEngine{delays: tc.delays}
			for nodeID := range tc.delays {
				insertPostgresEvaluationTestNode(t, g, nodeID, nodeID, int64(100+len(nodeID)))
			}
			cfg := defaultAccessProfileConfig("profile_pg_" + strings.ReplaceAll(tc.name, " ", "_"))
			cfg.Name = "pg " + tc.name
			cfg.Type = "fastest"
			cfg.TestURL = "http://example.test/probe"
			cfg.CurrentNodeID = tc.currentNodeID
			cfg.State = "ready"
			cfg.MinEvaluationIntervalSeconds = 0
			cfg.RelativeImprovementThreshold = 0
			cfg.AbsoluteLatencyImprovementMS = 1
			if err := g.insertAccessProfileConfig(cfg); err != nil {
				t.Fatal(err)
			}
			runID, err := g.enqueueProfileEvaluationRun(cfg.ID, cfg.Name, maintenanceapp.TriggerManual, cfg.ConfigVersion, false)
			if err != nil {
				t.Fatal(err)
			}

			if err := g.runMaintenanceRun(runID); err != nil {
				t.Fatal(err)
			}

			run, err := g.loadMaintenanceRun(runID)
			if err != nil {
				t.Fatal(err)
			}
			if run.State != maintenanceRunStateFinished || run.Result != maintenanceRunResultSuccess || run.ReasonCode != tc.wantReason {
				t.Fatalf("run = %#v, want finished success %s", run, tc.wantReason)
			}
			cfg, err = g.loadAccessProfileConfig(cfg.ID)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.CurrentNodeID != tc.bestNodeID || cfg.SwitchReason != tc.wantReason {
				t.Fatalf("profile current/reason = %s/%s, want %s/%s", cfg.CurrentNodeID, cfg.SwitchReason, tc.bestNodeID, tc.wantReason)
			}
			detail := maintenanceRunDetail(run)
			if detail["switch_reason"] != tc.wantReason || detail["selected_node_id"] != tc.bestNodeID {
				t.Fatalf("run detail = %#v, want reason %s selected %s", detail, tc.wantReason, tc.bestNodeID)
			}
		})
	}
}

func TestPostgresCurrentNodeRemovalKeepsStableSwitchReason(t *testing.T) {
	t.Parallel()

	g, _ := newObservedPostgresGatewayForInternalTest(t)
	insertPostgresEvaluationTestNode(t, g, "node_pg_removed_current", "pg-removed-current", 100)
	cfg := defaultAccessProfileConfig("profile_pg_removed_current")
	cfg.Name = "pg removed current"
	cfg.Type = "fastest"
	cfg.CurrentNodeID = "node_pg_removed_current"
	cfg.State = "ready"
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}

	result, err := appnodes.DeleteService{Runner: g.txRunners}.DeleteManualSource(context.Background(), "node_pg_removed_current")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DeletedFingerprints) != 1 {
		t.Fatalf("delete result = %#v, want one runtime fingerprint", result)
	}

	cfg, err = g.loadAccessProfileConfig(cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.State != "waiting_observation" || cfg.CurrentNodeID != "" || cfg.SwitchReason != "current_node_removed" {
		t.Fatalf("profile after current node removal = state=%q current=%q reason=%q, want waiting_observation empty current_node_removed", cfg.State, cfg.CurrentNodeID, cfg.SwitchReason)
	}
}

func newObservedPostgresGatewayForInternalTest(t *testing.T) (*Gateway, *observer.ObservedLogs) {
	t.Helper()

	core, observed := observer.New(zapcore.DebugLevel)
	g, err := New(t.TempDir(),
		WithLogger(zap.New(core)),
		WithoutMaintenanceRunner(),
		WithStorageConfig(storageinfra.Config{
			Driver: storageinfra.DriverPostgres,
			DSN:    isolatedPostgresDSNForInternalAppTest(t),
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g, observed
}

func isolatedPostgresDSNForInternalAppTest(t *testing.T) string {
	t.Helper()

	rawDSN := strings.TrimSpace(os.Getenv("PROXYGATEWAY_TEST_POSTGRES_DSN"))
	if rawDSN == "" {
		t.Skip("PROXYGATEWAY_TEST_POSTGRES_DSN is not set")
	}
	base, err := sql.Open(postgresinfra.DriverName, rawDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })

	schema := fmt.Sprintf("proxygateway_internal_app_test_%d", time.Now().UnixNano())
	if _, err := base.ExecContext(context.Background(), `CREATE SCHEMA `+quoteIdentForInternalAppPostgresTest(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = base.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+quoteIdentForInternalAppPostgresTest(schema)+` CASCADE`)
	})

	config, err := pgx.ParseConfig(rawDSN)
	if err != nil {
		t.Fatal(err)
	}
	if config.RuntimeParams == nil {
		config.RuntimeParams = map[string]string{}
	}
	config.RuntimeParams["search_path"] = schema
	connString := stdlib.RegisterConnConfig(config)
	t.Cleanup(func() {
		stdlib.UnregisterConnConfig(connString)
	})
	return connString
}

func quoteIdentForInternalAppPostgresTest(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func insertPostgresEvaluationTestNode(t *testing.T, g *Gateway, id, name string, createdAt int64) {
	t.Helper()
	if _, err := g.store.DB.Exec(
		`INSERT INTO nodes (id, fingerprint, name, type, created_at) VALUES ($1, $2, $3, 'direct', $4)`,
		id,
		"fp-"+id,
		name,
		createdAt,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := g.store.DB.Exec(
		`INSERT INTO node_sources (node_id, source_id, source_name, source_type, display_name, created_at)
		 VALUES ($1, 'manual', 'Manual', 'manual', $2, $3)`,
		id,
		name,
		createdAt,
	); err != nil {
		t.Fatal(err)
	}
}

type postgresEvaluationTestEngine struct {
	failingNodeID string
	delays        map[string]time.Duration
}

func (e postgresEvaluationTestEngine) DialNode(node nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	if node.ID == e.failingNodeID {
		return nil, errors.New("candidate dial failed")
	}
	if delay := e.delays[node.ID]; delay > 0 {
		time.Sleep(delay)
	}
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		_, _ = http.ReadRequest(bufio.NewReader(server))
		_, _ = server.Write([]byte("HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n"))
	}()
	return client, nil
}

func (e postgresEvaluationTestEngine) DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	return e.DialNode(frontNode, target, timeouts)
}
