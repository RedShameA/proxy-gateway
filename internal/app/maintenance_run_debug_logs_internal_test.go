package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNodeObservationMaintenanceRunLogsDebugResults(t *testing.T) {
	t.Parallel()

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ip=203.0.113.10\nloc=US\n"))
	}))
	t.Cleanup(probe.Close)

	g, observed := newObservedGatewayForTest(t)
	g.protocolEngine = directTestEngine{}
	insertMaintenanceRunTestNode(t, g, "node_observation_debug", "observation-debug")
	run, err := g.createNodeObservationRun("manual", "single_node", []nodeRecord{{ID: "node_observation_debug", Name: "observation-debug", Enabled: true}}, probe.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := g.runNodeObservationMaintenanceRun(run.ID); err != nil {
		t.Fatal(err)
	}

	logs := observed.FilterMessage("node observation result").All()
	if len(logs) != 1 {
		t.Fatalf("node observation debug logs = %d, want 1: %#v", len(logs), logs)
	}
	fields := logs[0].ContextMap()
	assertStringField(t, fields, "run_id", run.ID)
	assertStringField(t, fields, "node_id", "node_observation_debug")
	assertStringField(t, fields, "node_name", "observation-debug")
	assertBoolField(t, fields, "success", true)
	assertStringField(t, fields, "error", "")
}

func TestProfileEvaluationMaintenanceRunLogsDebugCandidatesAndSelection(t *testing.T) {
	t.Parallel()

	g, observed := newObservedGatewayForTest(t)
	g.protocolEngine = mixedProfileEvaluationEngine{failingNodeID: "node_debug_bad"}
	insertMaintenanceRunTestNode(t, g, "node_debug_ok", "debug-ok")
	insertMaintenanceRunTestNode(t, g, "node_debug_bad", "debug-bad")
	cfg := defaultAccessProfileConfig("profile_debug_partial_candidates")
	cfg.Name = "debug partial candidates"
	cfg.Type = "fastest"
	cfg.TestURL = "http://example.test/probe"
	cfg.MinEvaluationIntervalSeconds = 0
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	runID, err := g.enqueueProfileEvaluationRun(cfg.ID, cfg.Name, "manual", cfg.ConfigVersion, true)
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

	candidateLogs := observed.FilterMessage("profile candidate probe result").All()
	if len(candidateLogs) != 2 {
		t.Fatalf("candidate debug logs = %d, want 2: %#v", len(candidateLogs), candidateLogs)
	}
	okLog := observedLogWithStringField(t, candidateLogs, "node_id", "node_debug_ok")
	okFields := okLog.ContextMap()
	assertStringField(t, okFields, "profile_id", cfg.ID)
	assertStringField(t, okFields, "node_name", "debug-ok")
	assertBoolField(t, okFields, "success", true)
	assertIntField(t, okFields, "http_status", 204)
	assertStringField(t, okFields, "error", "")

	badLog := observedLogWithStringField(t, candidateLogs, "node_id", "node_debug_bad")
	badFields := badLog.ContextMap()
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
	assertStringField(t, selectionFields, "best_node_id", "node_debug_ok")
	assertStringField(t, selectionFields, "selected_node_id", "node_debug_ok")
	assertStringField(t, selectionFields, "switch_reason", "initial_selection")
}

func TestChainLinkEvaluationDebugCandidateOmitsHTTPStatus(t *testing.T) {
	t.Parallel()

	g, observed := newObservedGatewayForTest(t)
	g.protocolEngine = mixedProfileEvaluationEngine{}
	insertMaintenanceRunTestNode(t, g, "node_chain_debug_front", "chain-debug-front")
	insertMaintenanceRunTestNode(t, g, "node_chain_debug_exit", "chain-debug-exit")
	cfg := defaultAccessProfileConfig("profile_chain_link_debug")
	cfg.Name = "chain link debug"
	cfg.Type = "chain"
	cfg.ExitNodeIDs = []string{"node_chain_debug_exit"}
	cfg.ChainEvaluationMode = "chain_link"
	cfg.TestURL = "http://example.test/not-used"
	cfg.MinEvaluationIntervalSeconds = 0
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	target, settings, skipped, err := g.profileEvaluationTarget(cfg.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if skipped {
		t.Fatal("profile evaluation target skipped")
	}
	settings.GlobalConcurrency = 1

	if ok := g.evaluateFastestFrontProfile(target, settings); !ok {
		t.Fatal("chain-link evaluation failed, want success")
	}

	logs := observed.FilterMessage("chain candidate probe result").All()
	if len(logs) != 1 {
		t.Fatalf("chain candidate debug logs = %d, want 1: %#v", len(logs), logs)
	}
	fields := logs[0].ContextMap()
	assertStringField(t, fields, "profile_id", cfg.ID)
	assertStringField(t, fields, "front_node_id", "node_chain_debug_front")
	assertStringField(t, fields, "front_node_name", "chain-debug-front")
	assertStringField(t, fields, "exit_node_id", "node_chain_debug_exit")
	assertStringField(t, fields, "exit_node_name", "chain-debug-exit")
	assertBoolField(t, fields, "success", true)
	assertStringField(t, fields, "error", "")
	assertFieldAbsent(t, fields, "http_status")
}

func TestSubscriptionRefreshMaintenanceRunLogsDebugSummaryWithoutRawContent(t *testing.T) {
	t.Parallel()

	g, observed := newObservedGatewayForTest(t)
	content := `
proxies:
  - name: clash-http-ok
    type: http
    server: 127.0.0.1
    port: 18084
  - name: clash-missing-server
    type: http
    port: 18085
proxy-groups:
  - name: auto
    type: url-test
    proxies:
      - clash-http-ok
`
	sub := subscriptionRecord{
		ID:                 "sub_debug_refresh",
		Name:               "debug refresh",
		SourceType:         "local",
		Content:            content,
		AutoRefreshEnabled: true,
	}
	if _, err := g.createSubscriptionWithContent(sub, content); err != nil {
		t.Fatal(err)
	}
	runID, err := g.enqueueSubscriptionRefreshRun(sub.ID, sub.Name, "manual")
	if err != nil {
		t.Fatal(err)
	}

	if err := g.runSubscriptionRefreshMaintenanceRun(runID); err != nil {
		t.Fatal(err)
	}

	entrySummaryLogs := observed.FilterMessage("subscription refresh entry summary").All()
	if len(entrySummaryLogs) != 2 {
		t.Fatalf("subscription entry summary logs = %d, want 2: %#v", len(entrySummaryLogs), entrySummaryLogs)
	}
	ignoredLog := observedLogWithStringField(t, entrySummaryLogs, "summary_type", "ignored")
	ignoredFields := ignoredLog.ContextMap()
	assertStringField(t, ignoredFields, "run_id", runID)
	assertStringField(t, ignoredFields, "subscription_id", sub.ID)
	assertStringField(t, ignoredFields, "reason", "clash_proxy_group_ignored")
	assertIntField(t, ignoredFields, "count", 1)

	skippedLog := observedLogWithStringField(t, entrySummaryLogs, "summary_type", "skipped")
	skippedFields := skippedLog.ContextMap()
	assertStringField(t, skippedFields, "run_id", runID)
	assertStringField(t, skippedFields, "subscription_id", sub.ID)
	assertStringField(t, skippedFields, "reason", "missing_required_field")
	assertIntField(t, skippedFields, "count", 1)

	summaryLogs := observed.FilterMessage("subscription refresh summary").All()
	if len(summaryLogs) != 1 {
		t.Fatalf("subscription refresh summary logs = %d, want 1: %#v", len(summaryLogs), summaryLogs)
	}
	summaryFields := summaryLogs[0].ContextMap()
	assertStringField(t, summaryFields, "run_id", runID)
	assertStringField(t, summaryFields, "subscription_id", sub.ID)
	assertIntField(t, summaryFields, "imported_count", 1)
	assertIntField(t, summaryFields, "ignored_count", 1)
	assertIntField(t, summaryFields, "skipped_count", 1)
	assertStringField(t, summaryFields, "result", maintenanceRunResultSuccess)
	assertStringField(t, summaryFields, "reason_code", maintenanceRunReasonCompleted)

	assertObservedLogsDoNotContain(t, observed.All(), "clash-http-ok")
	assertObservedLogsDoNotContain(t, observed.All(), "clash-missing-server")
	assertObservedLogsDoNotContain(t, observed.All(), "server: 127.0.0.1")
}

func newObservedGatewayForTest(t *testing.T) (*Gateway, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zapcore.DebugLevel)
	g, err := New(t.TempDir(), WithLogger(zap.New(core)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g, observed
}

func observedLogWithStringField(t *testing.T, logs []observer.LoggedEntry, key, value string) observer.LoggedEntry {
	t.Helper()
	for _, entry := range logs {
		if got, ok := entry.ContextMap()[key].(string); ok && got == value {
			return entry
		}
	}
	t.Fatalf("log with %s=%q not found in %#v", key, value, logs)
	return observer.LoggedEntry{}
}

func assertStringField(t *testing.T, fields map[string]any, key, want string) {
	t.Helper()
	got, ok := fields[key].(string)
	if !ok || got != want {
		t.Fatalf("field %s = %#v, want %q", key, fields[key], want)
	}
}

func assertBoolField(t *testing.T, fields map[string]any, key string, want bool) {
	t.Helper()
	got, ok := fields[key].(bool)
	if !ok || got != want {
		t.Fatalf("field %s = %#v, want %t", key, fields[key], want)
	}
}

func assertIntField(t *testing.T, fields map[string]any, key string, want int64) {
	t.Helper()
	got, ok := observedIntField(fields[key])
	if !ok || got != want {
		t.Fatalf("field %s = %#v, want %d", key, fields[key], want)
	}
}

func assertFieldAbsent(t *testing.T, fields map[string]any, key string) {
	t.Helper()
	if _, ok := fields[key]; ok {
		t.Fatalf("field %s = %#v, want absent", key, fields[key])
	}
}

func observedIntField(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case int32:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case uint:
		return int64(typed), true
	case uint64:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	default:
		return 0, false
	}
}

func assertObservedLogsDoNotContain(t *testing.T, logs []observer.LoggedEntry, needle string) {
	t.Helper()
	for _, entry := range logs {
		if strings.Contains(entry.Message, needle) {
			t.Fatalf("log message %q contains %q", entry.Message, needle)
		}
		for key, value := range entry.ContextMap() {
			text, ok := value.(string)
			if ok && strings.Contains(text, needle) {
				t.Fatalf("log field %s=%q contains %q", key, text, needle)
			}
		}
	}
}
