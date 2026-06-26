package app_test

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"proxygateway/internal/app"
	"testing"
	"time"
)

func TestImportSingboxJSONSubscriptionCreatesNodes(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "local-singbox",
		"source_type": "local",
		"content": `{"outbounds":[
			{"type":"http","tag":"http-one","server":"127.0.0.1","server_port":18080},
			{"type":"direct","tag":"skip-direct"}
		]}`,
	})
	var sub struct {
		ID             string `json:"id"`
		ImportedNodes  int    `json:"imported_nodes"`
		SkippedEntries int    `json:"skipped_entries"`
	}
	decodeOK(t, subResp, &sub)
	if sub.ImportedNodes != 1 {
		t.Fatalf("imported_nodes = %d, want 1", sub.ImportedNodes)
	}
	if sub.SkippedEntries != 1 {
		t.Fatalf("skipped_entries = %d, want 1", sub.SkippedEntries)
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	found := false
	for _, node := range nodes.Nodes {
		if node["name"] == "http-one" && node["type"] == "http" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("imported node not found in nodes list: %#v", nodes.Nodes)
	}

	arrayResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "local-singbox-array",
		"source_type": "local",
		"content":     `[{"type":"http","tag":"http-array","server":"127.0.0.1","server_port":18089}]`,
	})
	var arraySub struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, arrayResp, &arraySub)
	if arraySub.ImportedNodes != 1 || arraySub.SkippedEntries != 0 {
		t.Fatalf("raw outbound array import = %+v, want 1 imported and 0 skipped", arraySub)
	}
}

func TestImportClashURIAndBase64SubscriptionFormats(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	clashYAML := `
proxies:
  - name: clash-http
    type: http
    server: 127.0.0.1
    port: 18081
  - name: unsupported-direct
    type: direct
`
	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "clash",
		"source_type": "local",
		"content":     clashYAML,
	})
	var clashSub struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, resp, &clashSub)
	if clashSub.ImportedNodes != 1 || clashSub.SkippedEntries != 1 {
		t.Fatalf("clash import = %+v, want 1 imported and 1 skipped", clashSub)
	}

	uriText := "socks5://user:pass@127.0.0.1:18082#uri-socks\nhttp://user:pass@127.0.0.1:18083#uri-http"
	encoded := base64.StdEncoding.EncodeToString([]byte(uriText))
	resp = postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "base64-uri",
		"source_type": "local",
		"content":     encoded,
	})
	var uriSub struct {
		ImportedNodes int `json:"imported_nodes"`
	}
	decodeOK(t, resp, &uriSub)
	if uriSub.ImportedNodes != 2 {
		t.Fatalf("uri imported_nodes = %d, want 2", uriSub.ImportedNodes)
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	names := map[string]bool{}
	for _, node := range nodes.Nodes {
		names[node["name"].(string)] = true
	}
	if !names["clash-http"] || !names["uri-socks"] || !names["uri-http"] {
		t.Fatalf("expected clash-http and uri-socks nodes, got %#v", names)
	}
}

func TestSubscriptionSkippedEntrySummaryIsReturnedAndPersisted(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "strict-singbox",
		"source_type": "local",
		"content": `{"outbounds":[
			{"type":"http","tag":"ok","server":"127.0.0.1","server_port":18083},
			{"type":"block","tag":"blocked"},
			{"type":"selector","tag":"select","outbounds":["ok"]},
			{"type":"http","tag":"missing-port","server":"127.0.0.1"},
			"not an outbound object"
		]}`,
	})
	var created struct {
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
	}
	decodeOK(t, resp, &created)
	if created.ImportedNodes != 1 {
		t.Fatalf("imported_nodes = %d, want 1", created.ImportedNodes)
	}
	if created.SkippedEntries != 4 {
		t.Fatalf("skipped_entries = %d, want 4", created.SkippedEntries)
	}
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "unsupported_functional_outbound", 2)
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "missing_required_field", 1)
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "malformed_entry", 1)

	var listed struct {
		Subscriptions []struct {
			Name                string                   `json:"name"`
			SkippedEntries      int                      `json:"skipped_entries"`
			SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
		} `json:"subscriptions"`
	}
	getJSON(t, srv.URL+"/api/subscriptions", adminToken, &listed)
	if len(listed.Subscriptions) != 1 {
		t.Fatalf("subscriptions = %d, want 1", len(listed.Subscriptions))
	}
	assertSkippedReasonCount(t, listed.Subscriptions[0].SkippedEntrySummary, "unsupported_functional_outbound", 2)
	assertSkippedReasonCount(t, listed.Subscriptions[0].SkippedEntrySummary, "missing_required_field", 1)
	assertSkippedReasonCount(t, listed.Subscriptions[0].SkippedEntrySummary, "malformed_entry", 1)

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	if len(nodes.Nodes) != 1 || nodes.Nodes[0]["name"] != "ok" {
		t.Fatalf("functional and invalid outbounds must not create nodes: %#v", nodes.Nodes)
	}
}

func TestSubscriptionRefreshUpdatesImportResultAndKeepsNodesOnFailure(t *testing.T) {
	t.Parallel()

	content := `{"outbounds":[
		{"type":"http","tag":"remote-stale","server":"127.0.0.1","server_port":18085},
		{"type":"http","tag":"remote-one","server":"127.0.0.1","server_port":18086}
	]}`
	failFetch := false
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failFetch {
			http.Error(w, "temporary provider failure", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(remote.Close)

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "remote-sub",
		"source_type": "remote",
		"url":         remote.URL,
	})
	var created struct {
		ID             string `json:"id"`
		ImportedNodes  int    `json:"imported_nodes"`
		SkippedEntries int    `json:"skipped_entries"`
	}
	decodeOK(t, resp, &created)
	if created.ImportedNodes != 2 || created.SkippedEntries != 0 {
		t.Fatalf("initial import = %+v, want 2 imported and 0 skipped", created)
	}

	content = `{"outbounds":[
		{"type":"http","tag":"remote-one","server":"127.0.0.1","server_port":18086},
		{"type":"http","tag":"remote-two","server":"127.0.0.1","server_port":18087},
		{"type":"selector","tag":"remote-group","outbounds":["remote-one"]}
	]}`
	refreshResp := postJSON(t, srv.URL+"/api/subscriptions/"+created.ID+"/refresh", adminToken, map[string]any{})
	var refreshed struct {
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
	}
	decodeOK(t, refreshResp, &refreshed)
	if refreshed.ImportedNodes != 2 || refreshed.SkippedEntries != 1 {
		t.Fatalf("refresh import = %+v, want 2 imported and 1 skipped", refreshed)
	}
	assertSkippedReasonCount(t, refreshed.SkippedEntrySummary, "unsupported_functional_outbound", 1)

	var nodesAfterSuccess struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodesAfterSuccess)
	names := map[string]bool{}
	for _, node := range nodesAfterSuccess.Nodes {
		names[node["name"].(string)] = true
	}
	if len(nodesAfterSuccess.Nodes) != 2 || names["remote-stale"] || !names["remote-one"] || !names["remote-two"] {
		t.Fatalf("refreshed nodes missing: %#v", names)
	}

	failFetch = true
	failedRefreshResp := postJSON(t, srv.URL+"/api/subscriptions/"+created.ID+"/refresh", adminToken, map[string]any{})
	if failedRefreshResp.StatusCode != http.StatusBadGateway {
		t.Fatalf("failed refresh status = %d, want 502", failedRefreshResp.StatusCode)
	}
	_ = failedRefreshResp.Body.Close()

	var nodesAfterFailure struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodesAfterFailure)
	if len(nodesAfterFailure.Nodes) != len(nodesAfterSuccess.Nodes) {
		t.Fatalf("refresh failure should keep existing nodes: before=%d after=%d", len(nodesAfterSuccess.Nodes), len(nodesAfterFailure.Nodes))
	}

	var listed struct {
		Subscriptions []struct {
			ID        string `json:"id"`
			LastError string `json:"last_error"`
		} `json:"subscriptions"`
	}
	getJSON(t, srv.URL+"/api/subscriptions", adminToken, &listed)
	if len(listed.Subscriptions) != 1 || listed.Subscriptions[0].LastError == "" {
		t.Fatalf("failed refresh should persist last_error: %#v", listed.Subscriptions)
	}
}

func TestSubscriptionDetailOmitsRemoteContent(t *testing.T) {
	t.Parallel()

	content := `{"outbounds":[{"type":"http","tag":"remote-detail-node","server":"127.0.0.1","server_port":18092}]}`
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(remote.Close)

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	remoteResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "remote-detail",
		"source_type": "remote",
		"url":         remote.URL,
	})
	var remoteCreated struct {
		ID string `json:"id"`
	}
	decodeOK(t, remoteResp, &remoteCreated)

	var remoteDetail map[string]any
	getJSON(t, srv.URL+"/api/subscriptions/"+remoteCreated.ID, adminToken, &remoteDetail)
	if _, ok := remoteDetail["content"]; ok {
		t.Fatalf("remote subscription detail must omit content: %#v", remoteDetail)
	}

	localContent := `{"outbounds":[{"type":"http","tag":"local-detail-node","server":"127.0.0.1","server_port":18093}]}`
	localResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "local-detail",
		"source_type": "local",
		"content":     localContent,
	})
	var localCreated struct {
		ID string `json:"id"`
	}
	decodeOK(t, localResp, &localCreated)

	var localDetail map[string]any
	getJSON(t, srv.URL+"/api/subscriptions/"+localCreated.ID, adminToken, &localDetail)
	if localDetail["content"] != localContent {
		t.Fatalf("local subscription detail content = %#v, want %q", localDetail["content"], localContent)
	}
}

func TestSubscriptionRefreshCurrentDynamicProfileNodeStartsReevaluation(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("dynamic profile refresh"))
	}))
	t.Cleanup(target.Close)

	selectedProxy := newDelayedHTTPConnectProxy(t, 0)
	survivorProxy := newDelayedHTTPConnectProxy(t, 120*time.Millisecond)
	content := fmt.Sprintf(`{"outbounds":[
		{"type":"http","tag":"refresh-selected-stale","server":"127.0.0.1","server_port":%d},
		{"type":"http","tag":"refresh-survivor","server":"127.0.0.1","server_port":%d}
	]}`, selectedProxy.port, survivorProxy.port)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(remote.Close)

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "dynamic-refresh-sub",
		"source_type": "remote",
		"url":         remote.URL,
	})
	var sub struct {
		ID string `json:"id"`
	}
	decodeOK(t, subResp, &sub)
	selectedNodeID := nodeIDByName(t, srv.URL, adminToken, "refresh-selected-stale")

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "dynamic-refresh-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, selectedNodeID, "ready")

	content = fmt.Sprintf(`{"outbounds":[
		{"type":"http","tag":"refresh-survivor","server":"127.0.0.1","server_port":%d}
	]}`, survivorProxy.port)
	decodeOK(t, postJSON(t, srv.URL+"/api/subscriptions/"+sub.ID+"/refresh", adminToken, map[string]any{}), &struct{}{})

	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, "", "waiting_observation")
	var runs struct {
		Items []struct {
			RunType  string         `json:"run_type"`
			TargetID string         `json:"target_id"`
			Trigger  string         `json:"trigger_source"`
			State    string         `json:"state"`
			Detail   map[string]any `json:"detail"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=profile_evaluation&target_id="+profile.ID, adminToken, &runs)
	for _, run := range runs.Items {
		if run.Trigger == "current_node_observed" {
			t.Fatalf("current-node-observed run before subscription observation = %#v, want none", runs.Items)
		}
	}

	gw.MaintenanceForTest().RunQueuedTasksForTest()
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=profile_evaluation&target_id="+profile.ID, adminToken, &runs)
	if len(runs.Items) == 0 {
		t.Fatalf("profile evaluation run was not queued after subscription observation")
	}
	var observedRun *struct {
		RunType  string         `json:"run_type"`
		TargetID string         `json:"target_id"`
		Trigger  string         `json:"trigger_source"`
		State    string         `json:"state"`
		Detail   map[string]any `json:"detail"`
	}
	for i := range runs.Items {
		if runs.Items[i].Trigger == "current_node_observed" {
			observedRun = &runs.Items[i]
			break
		}
	}
	if observedRun == nil || observedRun.RunType != "profile_evaluation" || observedRun.TargetID != profile.ID {
		t.Fatalf("current-node-observed maintenance run not found: %#v", runs.Items)
	}
	if observedRun.Detail["force_switch"] != true {
		t.Fatalf("current-node-observed run detail = %#v, want force_switch", observedRun.Detail)
	}
}

func TestSubscriptionRefreshStickyProfileRetainsHiddenCurrentNodeUntilSwitch(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("sticky profile target"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	stickyProxy := newDelayedHTTPConnectProxy(t, 0)
	replacementProxy := newDelayedHTTPConnectProxy(t, 40*time.Millisecond)
	content := fmt.Sprintf(`{"outbounds":[
		{"type":"http","tag":"sticky-current","server":"127.0.0.1","server_port":%d}
	]}`, stickyProxy.port)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(remote.Close)

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluation-settings", adminToken, map[string]any{
		"global_concurrency":                      4,
		"default_min_evaluation_interval_seconds": 0,
		"single_candidate_limit":                  0,
		"chain_candidate_limit":                   100,
	}), &struct{}{})

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "sticky-refresh-sub",
		"source_type": "remote",
		"url":         remote.URL,
	})
	var sub struct {
		ID string `json:"id"`
	}
	decodeOK(t, subResp, &sub)
	stickyNodeID := nodeIDByName(t, srv.URL, adminToken, "sticky-current")

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                "sticky-fastest",
		"type":                "fastest",
		"test_url":            target.URL,
		"node_sticky_enabled": true,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, stickyNodeID, "ready")

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "sticky-client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	content = `{"outbounds":[]}`
	decodeOK(t, postJSON(t, srv.URL+"/api/subscriptions/"+sub.ID+"/refresh", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, stickyNodeID, "degraded")
	assertProfileSwitchReason(t, srv.URL, adminToken, profile.ID, "current_node_removed")
	assertNodeVisible(t, srv.URL, adminToken, stickyNodeID, false)
	assertProfileEvaluationRun(t, srv.URL, adminToken, profile.ID, "current_node_removed", true)

	beforeStickyConnects := stickyProxy.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/sticky")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if stickyProxy.connects <= beforeStickyConnects {
		t.Fatal("sticky retained node did not continue serving proxy requests")
	}

	gw.MaintenanceForTest().RunQueuedTasksForTest()
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, stickyNodeID, "degraded")
	assertNodeVisible(t, srv.URL, adminToken, stickyNodeID, false)
	resp = get(t, srv.URL+"/api/nodes/"+stickyNodeID, adminToken)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retained node detail status = %d, want 200 after failed automatic switch", resp.StatusCode)
	}

	content = fmt.Sprintf(`{"outbounds":[
		{"type":"http","tag":"sticky-replacement","server":"127.0.0.1","server_port":%d}
	]}`, replacementProxy.port)
	decodeOK(t, postJSON(t, srv.URL+"/api/subscriptions/"+sub.ID+"/refresh", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, stickyNodeID, "degraded")
	replacementNodeID := nodeIDByName(t, srv.URL, adminToken, "sticky-replacement")
	assertProfileEvaluationRun(t, srv.URL, adminToken, profile.ID, "current_node_removed", true)
	gw.MaintenanceForTest().RunQueuedTasksForTest()
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, replacementNodeID, "ready")
	assertNodeVisible(t, srv.URL, adminToken, stickyNodeID, false)
	resp = get(t, srv.URL+"/api/nodes/"+stickyNodeID, adminToken)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("retained node detail status = %d, want 404 after successful switch", resp.StatusCode)
	}
}

func assertNodeVisible(t *testing.T, baseURL, adminToken, nodeID string, wantVisible bool) {
	t.Helper()
	var nodes struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	getJSON(t, baseURL+"/api/nodes", adminToken, &nodes)
	for _, node := range nodes.Nodes {
		if node.ID == nodeID {
			if !wantVisible {
				t.Fatalf("node %s is visible in node pool, want hidden", nodeID)
			}
			return
		}
	}
	if wantVisible {
		t.Fatalf("node %s is hidden in node pool, want visible", nodeID)
	}
}

func assertProfileEvaluationRun(t *testing.T, baseURL, adminToken, profileID, trigger string, forceSwitch bool) {
	t.Helper()
	var runs struct {
		Items []struct {
			RunType       string         `json:"run_type"`
			TargetID      string         `json:"target_id"`
			TriggerSource string         `json:"trigger_source"`
			State         string         `json:"state"`
			Detail        map[string]any `json:"detail"`
		} `json:"items"`
	}
	getJSON(t, baseURL+"/api/maintenance/runs?run_type=profile_evaluation&target_id="+profileID, adminToken, &runs)
	for _, run := range runs.Items {
		if run.RunType != "profile_evaluation" || run.TargetID != profileID || run.TriggerSource != trigger {
			continue
		}
		if run.Detail["force_switch"] != forceSwitch {
			t.Fatalf("profile evaluation run detail = %#v, want force_switch=%v", run.Detail, forceSwitch)
		}
		return
	}
	t.Fatalf("profile evaluation run trigger=%s not found: %#v", trigger, runs.Items)
}

func TestDeletingSubscriptionRemovesSourceWithoutDeletingSharedNode(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	manualResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-shared-delete",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 18088,
	})
	decodeOK(t, manualResp, &struct{}{})

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "delete-source-sub",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"http","tag":"subscription-shared-delete","server":"127.0.0.1","server_port":18088}]}`,
	})
	var sub struct {
		ID string `json:"id"`
	}
	decodeOK(t, subResp, &sub)

	var before struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &before)
	if len(before.Nodes) != 1 {
		t.Fatalf("node count before delete = %d, want deduped 1: %#v", len(before.Nodes), before.Nodes)
	}
	if sources := before.Nodes[0]["sources"].([]any); len(sources) != 2 {
		t.Fatalf("sources before delete = %d, want 2: %#v", len(sources), sources)
	}

	deleteResp := deleteRequest(t, srv.URL+"/api/subscriptions/"+sub.ID, adminToken)
	decodeOK(t, deleteResp, &struct{}{})

	var after struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &after)
	if len(after.Nodes) != 1 {
		t.Fatalf("shared node should remain after deleting subscription source: %#v", after.Nodes)
	}
	sources := after.Nodes[0]["sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("sources after delete = %d, want manual source only: %#v", len(sources), sources)
	}
	source := sources[0].(map[string]any)
	if source["source_type"] != "manual" {
		t.Fatalf("remaining source = %#v, want manual", source)
	}

	var subscriptions struct {
		Subscriptions []map[string]any `json:"subscriptions"`
	}
	getJSON(t, srv.URL+"/api/subscriptions", adminToken, &subscriptions)
	if len(subscriptions.Subscriptions) != 0 {
		t.Fatalf("subscription should be deleted: %#v", subscriptions.Subscriptions)
	}
}

func TestDeletingSubscriptionRemovesOrphanNodeAndInvalidatesReferences(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "orphan-source-sub",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"http","tag":"subscription-only-delete","server":"127.0.0.1","server_port":18091}]}`,
	})
	var sub struct {
		ID string `json:"id"`
	}
	decodeOK(t, subResp, &sub)
	nodeID := nodeIDByName(t, srv.URL, adminToken, "subscription-only-delete")

	fixedProfileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed-subscription-node",
		"type":          "fixed_node",
		"fixed_node_id": nodeID,
	})
	var fixedProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, fixedProfileResp, &fixedProfile)

	filteredProfileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":             "deleted-subscription-filter",
		"type":             "fastest",
		"test_url":         "http://example.test/",
		"node_source_mode": "specific_subscriptions",
		"source_ids":       []string{sub.ID},
	})
	var filteredProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, filteredProfileResp, &filteredProfile)

	deleteResp := deleteRequest(t, srv.URL+"/api/subscriptions/"+sub.ID, adminToken)
	decodeOK(t, deleteResp, &struct{}{})

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	for _, node := range nodes.Nodes {
		if node["id"] == nodeID {
			t.Fatalf("orphan node should be deleted after source deletion: %#v", nodes.Nodes)
		}
	}
	assertProfileCurrentNode(t, srv.URL, adminToken, fixedProfile.ID, "", "invalid_config")
	assertProfileCurrentNode(t, srv.URL, adminToken, filteredProfile.ID, "", "invalid_config")
}

func TestClashProxyGroupsAreSkippedWithoutCreatingAccessProfiles(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "clash-with-groups",
		"source_type": "local",
		"content": `
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
`,
	})
	var created struct {
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
	}
	decodeOK(t, resp, &created)
	if created.ImportedNodes != 1 {
		t.Fatalf("imported_nodes = %d, want 1", created.ImportedNodes)
	}
	if created.SkippedEntries != 2 {
		t.Fatalf("skipped_entries = %d, want 2", created.SkippedEntries)
	}
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "missing_required_field", 1)
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "clash_proxy_group_ignored", 1)

	var profiles struct {
		AccessProfiles []map[string]any `json:"access_profiles"`
	}
	getJSON(t, srv.URL+"/api/access-profiles", adminToken, &profiles)
	if len(profiles.AccessProfiles) != 0 {
		t.Fatalf("proxy-groups must not create access profiles: %#v", profiles.AccessProfiles)
	}
}

func assertSkippedReasonCount(t *testing.T, summary []map[string]interface{}, reason string, want int) {
	t.Helper()
	for _, item := range summary {
		if item["reason"] == reason {
			if int(item["count"].(float64)) != want {
				t.Fatalf("summary[%s] count = %v, want %d; summary=%#v", reason, item["count"], want, summary)
			}
			return
		}
	}
	t.Fatalf("summary missing reason %s: %#v", reason, summary)
}
