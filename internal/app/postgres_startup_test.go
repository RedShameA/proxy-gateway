package app_test

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"proxygateway/internal/app"
	maintenanceapp "proxygateway/internal/application/maintenance"
	postgresinfra "proxygateway/internal/infrastructure/postgres"
	storageinfra "proxygateway/internal/infrastructure/storage"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestPostgresGatewayStartsAndServesAdminAndSettings(t *testing.T) {
	t.Parallel()

	dsn := isolatedPostgresDSNForAppTest(t)
	gw, err := app.New(t.TempDir(),
		app.WithLogger(zap.NewNop()),
		app.WithoutMaintenanceRunner(),
		app.WithStorageConfig(storageinfra.Config{
			Driver: storageinfra.DriverPostgres,
			DSN:    dsn,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })

	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	var setupStatus struct {
		RequiresSetup bool `json:"requires_setup"`
	}
	getJSON(t, srv.URL+"/api/system/setup-status", "", &setupStatus)
	if !setupStatus.RequiresSetup {
		t.Fatal("fresh postgres store should require setup")
	}

	token := setupAdmin(t, srv.URL)
	var afterSetup struct {
		RequiresSetup bool `json:"requires_setup"`
	}
	getJSON(t, srv.URL+"/api/system/setup-status", "", &afterSetup)
	if afterSetup.RequiresSetup {
		t.Fatal("postgres store should not require setup after admin setup")
	}

	var overview struct {
		ResourceCounts map[string]any `json:"resource_counts"`
	}
	getJSON(t, srv.URL+"/api/overview", token, &overview)
	if overview.ResourceCounts == nil {
		t.Fatalf("overview resource_counts = nil")
	}

	var settings struct {
		PublicProxyEndpoint string `json:"public_proxy_endpoint"`
		Maintenance         struct {
			NodeObservationConcurrency int `json:"node_observation_concurrency"`
		} `json:"maintenance"`
		Evaluation struct {
			GlobalConcurrency int `json:"global_concurrency"`
		} `json:"evaluation"`
	}
	resp := patchJSON(t, srv.URL+"/api/system/settings", token, map[string]any{
		"public_proxy_endpoint": "127.0.0.1:8080",
		"maintenance": map[string]any{
			"node_observation_concurrency": 6,
		},
		"evaluation": map[string]any{
			"global_concurrency": 18,
		},
	})
	decodeOK(t, resp, &settings)
	if settings.PublicProxyEndpoint != "127.0.0.1:8080" {
		t.Fatalf("public_proxy_endpoint = %q", settings.PublicProxyEndpoint)
	}
	if settings.Maintenance.NodeObservationConcurrency != 6 {
		t.Fatalf("node_observation_concurrency = %d, want 6", settings.Maintenance.NodeObservationConcurrency)
	}
	if settings.Evaluation.GlobalConcurrency != 18 {
		t.Fatalf("global_concurrency = %d, want 18", settings.Evaluation.GlobalConcurrency)
	}
}

func TestPostgresGatewayMigrationFailureDoesNotLeakDSN(t *testing.T) {
	t.Parallel()

	dsn := isolatedPostgresDSNForAppTest(t)
	db, err := postgresinfra.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE nodes (id text PRIMARY KEY)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	_ = db.Close()

	core, logs := observer.New(zapcore.DebugLevel)
	_, err = app.New(t.TempDir(),
		app.WithLogger(zap.New(core)),
		app.WithoutMaintenanceRunner(),
		app.WithStorageConfig(storageinfra.Config{
			Driver: storageinfra.DriverPostgres,
			DSN:    dsn,
		}),
	)
	if err == nil {
		t.Fatal("expected conflicting postgres schema to fail migration")
	}
	if strings.Contains(err.Error(), dsn) {
		t.Fatalf("migration error leaked DSN secret: %v", err)
	}
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, dsn) {
			t.Fatalf("log message leaked DSN secret: %q", entry.Message)
		}
		for _, field := range entry.Context {
			if strings.Contains(field.String, dsn) {
				t.Fatalf("log field %q leaked DSN secret: %q", field.Key, field.String)
			}
		}
	}
}

func TestPostgresGatewayStartupCleanupCancelsActiveRuns(t *testing.T) {
	t.Parallel()

	dsn := isolatedPostgresDSNForAppTest(t)
	handle, err := storageinfra.Open(storageinfra.Config{Driver: storageinfra.DriverPostgres, DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	if err := storageinfra.Migrate(context.Background(), handle); err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	repo, err := storageinfra.NewMaintenanceRunRepository(handle)
	if err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	if err := repo.Insert(context.Background(), maintenanceapp.Run{
		ID:            "run_active_before_startup",
		RunType:       maintenanceapp.RunTypeProfileEvaluation,
		TriggerSource: maintenanceapp.TriggerScheduled,
		State:         maintenanceapp.StateQueued,
		CreatedAt:     1000,
		UpdatedAt:     1000,
	}); err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}

	gw, err := app.New(t.TempDir(),
		app.WithLogger(zap.NewNop()),
		app.WithoutMaintenanceRunner(),
		app.WithStorageConfig(storageinfra.Config{
			Driver: storageinfra.DriverPostgres,
			DSN:    dsn,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })

	handle, err = storageinfra.Open(storageinfra.Config{Driver: storageinfra.DriverPostgres, DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	repo, err = storageinfra.NewMaintenanceRunRepository(handle)
	if err != nil {
		t.Fatal(err)
	}
	run, err := repo.Load(context.Background(), "run_active_before_startup")
	if err != nil {
		t.Fatal(err)
	}
	if run.State != maintenanceapp.StateFinished || run.Result != maintenanceapp.ResultCancelled || run.ReasonCode != maintenanceapp.ReasonExpiredAfterRestart {
		t.Fatalf("active run after startup cleanup = %#v", run)
	}
}

func TestPostgresGatewayNodeObservationMaintenanceRunOutcomes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		nodes         []map[string]any
		probeStatus   int
		probeBody     string
		observedNodes int
		result        string
		reasonCode    string
		successCount  float64
		failureCount  float64
	}{
		{
			name: "success",
			nodes: []map[string]any{
				{"name": "pg-observation-success", "type": "direct"},
			},
			probeStatus:   http.StatusOK,
			probeBody:     "ip=203.0.113.51\nloc=US\n",
			observedNodes: 1,
			result:        maintenanceapp.ResultSuccess,
			reasonCode:    maintenanceapp.ReasonCompleted,
			successCount:  1,
			failureCount:  0,
		},
		{
			name: "partial failure",
			nodes: []map[string]any{
				{"name": "pg-observation-partial-success", "type": "direct"},
				{"name": "pg-observation-partial-failure", "type": "http", "server": "127.0.0.1", "server_port": 1},
			},
			probeStatus:   http.StatusOK,
			probeBody:     "ip=203.0.113.52\nloc=JP\n",
			observedNodes: 1,
			result:        maintenanceapp.ResultSuccess,
			reasonCode:    maintenanceapp.ReasonPartialFailure,
			successCount:  1,
			failureCount:  1,
		},
		{
			name: "all failed",
			nodes: []map[string]any{
				{"name": "pg-observation-all-failed", "type": "direct"},
			},
			probeStatus:   http.StatusBadGateway,
			probeBody:     "probe down",
			observedNodes: 0,
			result:        maintenanceapp.ResultFailure,
			reasonCode:    maintenanceapp.ReasonAllFailed,
			successCount:  0,
			failureCount:  1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.probeStatus)
				_, _ = w.Write([]byte(tc.probeBody))
			}))
			t.Cleanup(probe.Close)

			gw := newPostgresGatewayForAppTest(t)
			srv := httptest.NewServer(gw.Handler())
			t.Cleanup(srv.Close)
			adminToken := setupAdmin(t, srv.URL)

			for _, node := range tc.nodes {
				decodeOK(t, postJSON(t, srv.URL+"/api/nodes", adminToken, node), &struct{}{})
			}

			var runResp struct {
				ObservedNodes int `json:"observed_nodes"`
			}
			decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
				"probe_url": probe.URL,
			}), &runResp)
			if runResp.ObservedNodes != tc.observedNodes {
				t.Fatalf("observed_nodes = %d, want %d", runResp.ObservedNodes, tc.observedNodes)
			}

			run := latestMaintenanceRunForAppTest(t, srv.URL, adminToken, maintenanceapp.RunTypeNodeObservation)
			if run.State != maintenanceapp.StateFinished || run.Result != tc.result || run.ReasonCode != tc.reasonCode {
				t.Fatalf("run status = %#v, want finished %s %s", run, tc.result, tc.reasonCode)
			}
			if run.FinishedCount != run.TotalCount || run.TotalCount != len(tc.nodes) {
				t.Fatalf("run counts = %d/%d, want %d/%d", run.FinishedCount, run.TotalCount, len(tc.nodes), len(tc.nodes))
			}
			if run.Detail["success_count"] != tc.successCount || run.Detail["failure_count"] != tc.failureCount {
				t.Fatalf("run detail counts = %#v, want success=%v failure=%v", run.Detail, tc.successCount, tc.failureCount)
			}
			if tc.failureCount > 0 {
				if run.LastError == "" {
					t.Fatalf("last_error should be retained for failures: %#v", run)
				}
				if failures, ok := run.Detail["sample_failures"].([]any); !ok || len(failures) == 0 {
					t.Fatalf("sample_failures = %#v, want retained failure samples", run.Detail["sample_failures"])
				}
			}
		})
	}
}

func TestPostgresGatewaySubscriptionCRUDListPatchAndDelete(t *testing.T) {
	t.Parallel()

	gw := newPostgresGatewayForAppTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":                          "pg-local-sub",
		"source_type":                   "local",
		"content":                       `{"outbounds":[{"type":"http","tag":"pg-local-node","server":"127.0.0.1","server_port":18080},{"type":"selector","tag":"ignored","outbounds":["pg-local-node"]}]}`,
		"auto_refresh_enabled":          true,
		"auto_refresh_interval_seconds": 600,
	})
	var created struct {
		ID                  string                   `json:"id"`
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
	}
	decodeOK(t, createResp, &created)
	if created.ImportedNodes != 1 || created.SkippedEntries != 1 {
		t.Fatalf("created subscription import = %#v, want 1 imported and 1 skipped", created)
	}
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "unsupported_functional_outbound", 1)

	var listed struct {
		Subscriptions []struct {
			ID                         string                   `json:"id"`
			Name                       string                   `json:"name"`
			AutoRefreshEnabled         bool                     `json:"auto_refresh_enabled"`
			AutoRefreshIntervalSeconds int                      `json:"auto_refresh_interval_seconds"`
			SkippedEntrySummary        []map[string]interface{} `json:"skipped_entry_summary"`
		} `json:"subscriptions"`
	}
	getJSON(t, srv.URL+"/api/subscriptions", adminToken, &listed)
	if len(listed.Subscriptions) != 1 || listed.Subscriptions[0].ID != created.ID || listed.Subscriptions[0].Name != "pg-local-sub" {
		t.Fatalf("subscriptions list = %#v", listed.Subscriptions)
	}
	if !listed.Subscriptions[0].AutoRefreshEnabled || listed.Subscriptions[0].AutoRefreshIntervalSeconds != 600 {
		t.Fatalf("auto refresh list fields = %#v", listed.Subscriptions[0])
	}
	assertSkippedReasonCount(t, listed.Subscriptions[0].SkippedEntrySummary, "unsupported_functional_outbound", 1)

	var detail map[string]any
	getJSON(t, srv.URL+"/api/subscriptions/"+created.ID, adminToken, &detail)
	if detail["content"] == "" {
		t.Fatalf("local subscription detail should include content: %#v", detail)
	}

	decodeOK(t, patchJSON(t, srv.URL+"/api/subscriptions/"+created.ID, adminToken, map[string]any{
		"auto_refresh_enabled":          false,
		"auto_refresh_interval_seconds": 1200,
	}), &struct{}{})
	getJSON(t, srv.URL+"/api/subscriptions", adminToken, &listed)
	if listed.Subscriptions[0].AutoRefreshEnabled || listed.Subscriptions[0].AutoRefreshIntervalSeconds != 1200 {
		t.Fatalf("patched auto refresh fields = %#v", listed.Subscriptions[0])
	}

	deleteResp := deleteRequest(t, srv.URL+"/api/subscriptions/"+created.ID, adminToken)
	decodeOK(t, deleteResp, &struct{}{})
	getJSON(t, srv.URL+"/api/subscriptions", adminToken, &listed)
	if len(listed.Subscriptions) != 0 {
		t.Fatalf("subscription should be deleted: %#v", listed.Subscriptions)
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	if len(nodes.Nodes) != 0 {
		t.Fatalf("subscription-only node should be removed after delete: %#v", nodes.Nodes)
	}
}

func TestPostgresGatewayCreateSubscriptionQueuesImportObservationRun(t *testing.T) {
	t.Parallel()

	gw := newPostgresGatewayForAppTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "pg-import-observation",
		"source_type": "local",
		"content": `{"outbounds":[
			{"type":"http","tag":"pg-import-observation-node","server":"127.0.0.1","server_port":18080}
		]}`,
	})
	var created struct {
		ImportedNodes int `json:"imported_nodes"`
	}
	decodeOK(t, createResp, &created)
	if created.ImportedNodes != 1 {
		t.Fatalf("imported_nodes = %d, want 1", created.ImportedNodes)
	}

	var runs struct {
		Items []maintenanceRunForAppTest `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type="+maintenanceapp.RunTypeNodeObservation, adminToken, &runs)
	for _, run := range runs.Items {
		if run.TriggerSource == maintenanceapp.TriggerSubscriptionImport {
			if run.State != maintenanceapp.StateQueued || run.TotalCount != 1 {
				t.Fatalf("subscription import observation run = %#v, want queued total 1", run)
			}
			return
		}
	}
	t.Fatalf("node observation runs = %#v, want subscription_import run", runs.Items)
}

func TestPostgresGatewaySubscriptionRefreshRunWarnsWhenNoNodesImported(t *testing.T) {
	t.Parallel()

	content := `{"outbounds":[{"type":"http","tag":"pg-refresh-node","server":"127.0.0.1","server_port":18081}]}`
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(remote.Close)

	gw := newPostgresGatewayForAppTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "pg-refresh-sub",
		"source_type": "remote",
		"url":         remote.URL,
	})
	var created struct {
		ID            string `json:"id"`
		ImportedNodes int    `json:"imported_nodes"`
	}
	decodeOK(t, createResp, &created)
	if created.ImportedNodes != 1 {
		t.Fatalf("initial imported_nodes = %d, want 1", created.ImportedNodes)
	}
	var detail map[string]any
	getJSON(t, srv.URL+"/api/subscriptions/"+created.ID, adminToken, &detail)
	if _, ok := detail["content"]; ok {
		t.Fatalf("remote subscription detail must omit content: %#v", detail)
	}

	content = `{"outbounds":[
		{"type":"selector","tag":"ignored","outbounds":["missing"]},
		{"type":"http","tag":"missing-port","server":"127.0.0.1"}
	]}`
	refreshResp := postJSON(t, srv.URL+"/api/subscriptions/"+created.ID+"/refresh", adminToken, map[string]any{})
	var refreshed struct {
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
		Error               string                   `json:"error"`
	}
	decodeJSON(t, refreshResp, &refreshed)
	if refreshResp.StatusCode != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %#v, want 200", refreshResp.StatusCode, refreshed)
	}
	if refreshed.ImportedNodes != 0 || refreshed.SkippedEntries != 2 {
		t.Fatalf("refreshed import = %#v, want 0 imported and 2 skipped", refreshed)
	}
	assertSkippedReasonCount(t, refreshed.SkippedEntrySummary, "unsupported_functional_outbound", 1)
	assertSkippedReasonCount(t, refreshed.SkippedEntrySummary, "missing_required_field", 1)

	var runs struct {
		Items []maintenanceRunForAppTest `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type="+maintenanceapp.RunTypeSubscriptionRefresh+"&target_id="+created.ID, adminToken, &runs)
	if len(runs.Items) != 1 {
		t.Fatalf("subscription refresh runs = %#v, want one run", runs.Items)
	}
	run := runs.Items[0]
	if run.State != maintenanceapp.StateFinished || run.Result != maintenanceapp.ResultWarning || run.ReasonCode != maintenanceapp.ReasonNoImportableNodes {
		t.Fatalf("refresh run = %#v, want finished warning no_importable_nodes", run)
	}
	if run.Detail["imported_count"] != float64(0) || run.Detail["ignored_count"] != float64(1) || run.Detail["skipped_count"] != float64(1) {
		t.Fatalf("refresh run detail = %#v, want imported 0 ignored 1 skipped 1", run.Detail)
	}
}

func TestPostgresGatewaySubscriptionRefreshDebugLogsDoNotExposeRawContent(t *testing.T) {
	t.Parallel()

	content := `{"outbounds":[{"type":"http","tag":"pg-log-safe-node","server":"127.0.0.1","server_port":18082}]}`
	rawContent := `{"outbounds":[{"type":"selector","tag":"raw-secret-subscription-content","outbounds":["missing"]}]}`
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(remote.Close)

	core, observed := observer.New(zapcore.DebugLevel)
	gw := newPostgresGatewayForAppTest(t, app.WithLogger(zap.New(core)))
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "pg-log-safe-sub",
		"source_type": "remote",
		"url":         remote.URL,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	content = rawContent
	refreshResp := postJSON(t, srv.URL+"/api/subscriptions/"+created.ID+"/refresh", adminToken, map[string]any{})
	decodeOK(t, refreshResp, &struct{}{})

	for _, entry := range observed.All() {
		if strings.Contains(entry.Message, rawContent) || strings.Contains(entry.Message, "raw-secret-subscription-content") {
			t.Fatalf("log message leaked raw subscription content: %q", entry.Message)
		}
		for _, field := range entry.Context {
			if strings.Contains(field.String, rawContent) || strings.Contains(field.String, "raw-secret-subscription-content") {
				t.Fatalf("log field %q leaked raw subscription content: %q", field.Key, field.String)
			}
			if value := fmt.Sprint(field.Interface); strings.Contains(value, rawContent) || strings.Contains(value, "raw-secret-subscription-content") {
				t.Fatalf("log field %q leaked raw subscription content: %v", field.Key, value)
			}
		}
	}
}

func TestPostgresGatewayAccessProfileAndCredentialManagement(t *testing.T) {
	t.Parallel()

	gw := newPostgresGatewayForAppTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	frontResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "pg-front",
		"type": "direct",
	})
	var front struct {
		ID string `json:"id"`
	}
	decodeOK(t, frontResp, &front)
	exitResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "pg-exit",
		"type": "direct",
	})
	var exit struct {
		ID string `json:"id"`
	}
	decodeOK(t, exitResp, &exit)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":               "pg fixed",
		"profile_identifier": "pg-fixed",
		"type":               "fixed_node",
		"fixed_node_id":      front.ID,
	})
	var profile struct {
		ID                string `json:"id"`
		ProfileIdentifier string `json:"profile_identifier"`
		Type              string `json:"type"`
	}
	decodeOK(t, profileResp, &profile)
	if profile.ProfileIdentifier != "pg-fixed" || profile.Type != "fixed_node" {
		t.Fatalf("created profile = %#v", profile)
	}

	duplicateProfileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":               "pg duplicate",
		"profile_identifier": "pg-fixed",
		"type":               "fixed_node",
		"fixed_node_id":      front.ID,
	})
	var duplicateProfileBody map[string]any
	decodeJSON(t, duplicateProfileResp, &duplicateProfileBody)
	if duplicateProfileResp.StatusCode != http.StatusBadRequest || duplicateProfileBody["error"] != "策略标识已存在" {
		t.Fatalf("duplicate profile response = status %d body %#v, want 400 策略标识已存在", duplicateProfileResp.StatusCode, duplicateProfileBody)
	}

	decodeOK(t, patchJSON(t, srv.URL+"/api/access-profiles/"+profile.ID, adminToken, map[string]any{
		"name":                  "pg chain",
		"type":                  "chain",
		"exit_node_ids":         []string{exit.ID},
		"chain_evaluation_mode": "end_to_end",
		"test_url":              "https://example.com/generate_204",
		"candidate_filter": map[string]any{
			"source_mode":         "manual",
			"protocols":           []string{"direct"},
			"name_include":        "pg",
			"name_exclude":        "slow",
			"egress_country_mode": "exclude",
			"egress_countries":    []string{"cn"},
		},
		"candidate_limit":                  5,
		"min_evaluation_interval_seconds":  30,
		"auto_evaluation_enabled":          false,
		"auto_evaluation_interval_seconds": 900,
		"node_sticky_enabled":              true,
	}), &struct{}{})

	var detail map[string]any
	getJSON(t, srv.URL+"/api/access-profiles/"+profile.ID, adminToken, &detail)
	if detail["name"] != "pg chain" || detail["type"] != "chain" || detail["chain_evaluation_mode"] != "end_to_end" {
		t.Fatalf("profile detail stable codes = %#v", detail)
	}
	filter := detail["candidate_filter"].(map[string]any)
	if filter["source_mode"] != "manual" || filter["egress_country_mode"] != "exclude" {
		t.Fatalf("candidate filter stable codes = %#v", filter)
	}
	if detail["auto_evaluation_enabled"] != false || detail["node_sticky_enabled"] != true {
		t.Fatalf("profile detail booleans = %#v", detail)
	}

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "pg-client",
		"password": "proxy-password-123",
	})
	var credential struct {
		ID            string `json:"id"`
		Remark        string `json:"remark"`
		HTTPProxyURL  string `json:"http_proxy_url"`
		HTTPSProxyURL string `json:"https_proxy_url"`
		SOCKS5URL     string `json:"socks5_proxy_url"`
	}
	decodeOK(t, credentialResp, &credential)
	if credential.ID == "" || credential.Remark != "pg-client" || credential.HTTPProxyURL == "" || credential.HTTPSProxyURL == "" || credential.SOCKS5URL == "" {
		t.Fatalf("created credential = %#v", credential)
	}

	duplicateCredentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "pg-client-duplicate",
		"password": "proxy-password-123",
	})
	var duplicateCredentialBody map[string]any
	decodeJSON(t, duplicateCredentialResp, &duplicateCredentialBody)
	if duplicateCredentialResp.StatusCode != http.StatusConflict || duplicateCredentialBody["error"] != "该策略下已存在相同代理凭证密码" {
		t.Fatalf("duplicate credential response = status %d body %#v, want 409 duplicate password", duplicateCredentialResp.StatusCode, duplicateCredentialBody)
	}

	var credentials struct {
		ProxyCredentials []map[string]any `json:"proxy_credentials"`
	}
	getJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, &credentials)
	if len(credentials.ProxyCredentials) != 1 || credentials.ProxyCredentials[0]["id"] != credential.ID || credentials.ProxyCredentials[0]["password_hash"] != nil {
		t.Fatalf("credential list = %#v", credentials.ProxyCredentials)
	}

	decodeOK(t, patchJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials/"+credential.ID, adminToken, map[string]any{
		"enabled": false,
	}), &struct{}{})
	getJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, &credentials)
	if len(credentials.ProxyCredentials) != 1 || credentials.ProxyCredentials[0]["enabled"] != false {
		t.Fatalf("credential after disable = %#v", credentials.ProxyCredentials)
	}

	decodeOK(t, deleteRequest(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials/"+credential.ID, adminToken), &struct{}{})
	getJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, &credentials)
	if len(credentials.ProxyCredentials) != 0 {
		t.Fatalf("credential after delete = %#v", credentials.ProxyCredentials)
	}

	credentialResp = postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "pg-profile-delete",
		"password": "proxy-password-456",
	})
	decodeOK(t, credentialResp, &credential)
	decodeOK(t, deleteRequest(t, srv.URL+"/api/access-profiles/"+profile.ID, adminToken), &struct{}{})

	var profiles struct {
		AccessProfiles []map[string]any `json:"access_profiles"`
	}
	getJSON(t, srv.URL+"/api/access-profiles", adminToken, &profiles)
	for _, item := range profiles.AccessProfiles {
		if item["id"] == profile.ID {
			t.Fatalf("deleted profile still listed: %#v", profiles.AccessProfiles)
		}
	}
	missingCredentialsResp := get(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken)
	var missingCredentials map[string]any
	decodeJSON(t, missingCredentialsResp, &missingCredentials)
	if missingCredentialsResp.StatusCode != http.StatusNotFound || missingCredentials["error"] != "access profile not found" {
		t.Fatalf("missing credentials response = status %d body %#v", missingCredentialsResp.StatusCode, missingCredentials)
	}
}

func TestPostgresGatewayHTTPProxyWritesRequestLogs(t *testing.T) {
	t.Parallel()

	httpTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pg http request log"))
	}))
	t.Cleanup(httpTarget.Close)
	httpTargetURL, err := url.Parse(httpTarget.URL)
	if err != nil {
		t.Fatal(err)
	}
	httpsTarget := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pg connect request log"))
	}))
	t.Cleanup(httpsTarget.Close)
	httpsTargetURL, err := url.Parse(httpsTarget.URL)
	if err != nil {
		t.Fatal(err)
	}

	upstreamProxy := newHTTPConnectProxy(t)
	core, observed := observer.New(zapcore.DebugLevel)
	gw := newPostgresGatewayForAppTest(t, app.WithLogger(zap.New(core)))
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	credentialUser, credentialPassword := createFixedHTTPProxyAccess(t, srv.URL, adminToken, upstreamProxy)

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(credentialUser, credentialPassword)
	transport := &http.Transport{
		Proxy:           http.ProxyURL(proxyURL),
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transport}
	for _, target := range []string{"http://" + httpTargetURL.Host + "/pg-http-log", "https://" + httpsTargetURL.Host + "/pg-connect-log"} {
		resp, err := client.Get(target)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("target %s status = %d, want 200", target, resp.StatusCode)
		}
	}
	transport.CloseIdleConnections()

	for _, targetHost := range []string{httpTargetURL.Host, httpsTargetURL.Host} {
		logs := waitForPostgresRequestLogs(t, srv.URL+"/api/request-logs?access_profile_id="+url.QueryEscape(credentialUser)+"&target="+url.QueryEscape(targetHost)+"&success=true", adminToken, 1, observed)
		log := logs[0]
		if log["state"] != "completed" || log["result"] != "success" || log["success"] != true {
			t.Fatalf("request log status for %s = %#v, want completed success", targetHost, log)
		}
		if log["target_host"] != targetHost {
			t.Fatalf("target_host = %v, want %s", log["target_host"], targetHost)
		}
		if duration, ok := log["duration_ms"].(float64); !ok || duration <= 0 {
			t.Fatalf("duration_ms = %v, want positive", log["duration_ms"])
		}
		if _, ok := log["request_body"]; ok {
			t.Fatal("request log must not expose request_body")
		}
		if _, ok := log["response_body"]; ok {
			t.Fatal("request log must not expose response_body")
		}
	}

	badProxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	badProxyURL.User = url.UserPassword(credentialUser, "wrong-password")
	badClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(badProxyURL)}}
	resp, err := badClient.Get("http://" + httpTargetURL.Host + "/pg-auth-failure")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("auth failure status = %d, want 407", resp.StatusCode)
	}
	assertPostgresLatestRequestLogStage(t, srv.URL, adminToken, httpTargetURL.Host, "authentication", observed)
}

func TestPostgresGatewaySOCKS5RequestLogShowsRunningWhileTunnelOpen(t *testing.T) {
	t.Parallel()

	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = targetLn.Close() })
	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := targetLn.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()
	targetHost, targetPortText, err := net.SplitHostPort(targetLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var targetPort int
	if _, err := fmt.Sscanf(targetPortText, "%d", &targetPort); err != nil {
		t.Fatal(err)
	}

	gw := newPostgresGatewayForAppTest(t)
	baseURL, proxyAddr := startPostgresGatewayServeForAppTest(t, gw)
	adminToken := setupAdmin(t, baseURL)
	nodeResp := postJSON(t, baseURL+"/api/nodes", adminToken, map[string]any{
		"name": "pg-direct-running-log",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	profileResp := postJSON(t, baseURL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "pg-fixed-running-log",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, baseURL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "pg-running-log.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	conn, err := socks5Connect(proxyAddr, profile.ID, "proxy-password-123", targetHost, targetPort)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
		defer serverConn.Close()
	case err := <-acceptErr:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("target did not accept SOCKS5 tunnel")
	}

	runningURL := baseURL + "/api/request-logs?result=running&target=" + url.QueryEscape(targetLn.Addr().String())
	runningLogs := waitForPostgresRequestLogs(t, runningURL, adminToken, 1, nil)
	running := runningLogs[0]
	if running["state"] != "running" || running["result"] != "running" || running["success"] != nil {
		t.Fatalf("running request log = %#v, want state/result running and success nil", running)
	}
	if duration, ok := running["duration_ms"].(float64); !ok || duration <= 0 {
		t.Fatalf("running duration_ms = %v, want positive", running["duration_ms"])
	}

	_ = conn.Close()
	_ = serverConn.Close()
	completedURL := baseURL + "/api/request-logs?result=success&target=" + url.QueryEscape(targetLn.Addr().String())
	completedLogs := waitForPostgresRequestLogs(t, completedURL, adminToken, 1, nil)
	completed := completedLogs[0]
	if completed["state"] != "completed" || completed["result"] != "success" || completed["success"] != true {
		t.Fatalf("completed request log = %#v, want completed success", completed)
	}
	if duration, ok := completed["duration_ms"].(float64); !ok || duration <= 0 {
		t.Fatalf("completed duration_ms = %v, want positive", completed["duration_ms"])
	}
}

func newPostgresGatewayForAppTest(t *testing.T, opts ...app.Option) *app.Gateway {
	t.Helper()

	dsn := isolatedPostgresDSNForAppTest(t)
	options := []app.Option{
		app.WithLogger(zap.NewNop()),
		app.WithoutMaintenanceRunner(),
		app.WithStorageConfig(storageinfra.Config{
			Driver: storageinfra.DriverPostgres,
			DSN:    dsn,
		}),
	}
	options = append(options, opts...)
	gw, err := app.New(t.TempDir(), options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	return gw
}

func startPostgresGatewayServeForAppTest(t *testing.T, gw *app.Gateway) (string, string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		_ = gw.Serve(ln)
	}()
	addr := ln.Addr().String()
	return "http://" + addr, addr
}

func waitForPostgresRequestLogs(t *testing.T, endpoint string, token string, minCount int, observed *observer.ObservedLogs) []map[string]any {
	t.Helper()
	var logs struct {
		RequestLogs []map[string]any `json:"request_logs"`
	}
	for i := 0; i < 200; i++ {
		getJSON(t, endpoint, token, &logs)
		if len(logs.RequestLogs) >= minCount {
			return logs.RequestLogs
		}
		time.Sleep(10 * time.Millisecond)
	}
	var unfiltered struct {
		RequestLogs []map[string]any `json:"request_logs"`
	}
	if parsed, err := url.Parse(endpoint); err == nil {
		parsed.RawQuery = ""
		getJSON(t, parsed.String(), token, &unfiltered)
	}
	if observed != nil {
		t.Fatalf("request log count = %d, want at least %d; unfiltered = %#v; observed logs = %#v", len(logs.RequestLogs), minCount, unfiltered.RequestLogs, observed.All())
	}
	t.Fatalf("request log count = %d, want at least %d; unfiltered = %#v", len(logs.RequestLogs), minCount, unfiltered.RequestLogs)
	return nil
}

func assertPostgresLatestRequestLogStage(t *testing.T, baseURL, adminToken, target, wantStage string, observed *observer.ObservedLogs) {
	t.Helper()
	logs := waitForPostgresRequestLogs(t, baseURL+"/api/request-logs?result=failure&target="+url.QueryEscape(target), adminToken, 1, observed)
	if got := logs[0]["failure_stage"]; got != wantStage {
		t.Fatalf("failure_stage for %s = %v, want %s; log=%#v", target, got, wantStage, logs[0])
	}
}

type maintenanceRunForAppTest struct {
	RunType       string         `json:"run_type"`
	TriggerSource string         `json:"trigger_source"`
	TargetID      string         `json:"target_id"`
	State         string         `json:"state"`
	Result        string         `json:"result"`
	ReasonCode    string         `json:"reason_code"`
	TotalCount    int            `json:"total_count"`
	FinishedCount int            `json:"finished_count"`
	LastError     string         `json:"last_error"`
	Detail        map[string]any `json:"detail"`
}

func latestMaintenanceRunForAppTest(t *testing.T, baseURL, adminToken, runType string) maintenanceRunForAppTest {
	t.Helper()

	var body struct {
		Items []maintenanceRunForAppTest `json:"items"`
	}
	getJSON(t, baseURL+"/api/maintenance/runs?run_type="+runType, adminToken, &body)
	for _, run := range body.Items {
		if run.RunType == runType && run.TriggerSource == maintenanceapp.TriggerManual {
			return run
		}
	}
	t.Fatalf("manual %s run not found: %#v", runType, body.Items)
	return maintenanceRunForAppTest{}
}

func isolatedPostgresDSNForAppTest(t *testing.T) string {
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

	schema := fmt.Sprintf("proxygateway_app_test_%d", time.Now().UnixNano())
	if _, err := base.ExecContext(context.Background(), `CREATE SCHEMA `+quoteIdentForAppPostgresTest(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = base.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+quoteIdentForAppPostgresTest(schema)+` CASCADE`)
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

func quoteIdentForAppPostgresTest(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
