package app_test

import (
	"net/http"
	"net/http/httptest"
	"proxygateway/internal/testsupport/apptest"
	"testing"
)

func TestMaintenanceRunsExposeManualNodeObservationAndOverview(t *testing.T) {
	t.Parallel()

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ip=203.0.113.45\nloc=US\n"))
	}))
	t.Cleanup(probe.Close)

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "observed-maintenance-run-node",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)

	decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"probe_url": probe.URL,
		"node_id":   node.ID,
	}), &struct{}{})

	var runs struct {
		Items []struct {
			ID            string         `json:"id"`
			RunType       string         `json:"run_type"`
			TriggerSource string         `json:"trigger_source"`
			State         string         `json:"state"`
			Result        string         `json:"result"`
			ReasonCode    string         `json:"reason_code"`
			TotalCount    int            `json:"total_count"`
			FinishedCount int            `json:"finished_count"`
			Detail        map[string]any `json:"detail"`
			CreatedAt     int64          `json:"created_at"`
			FinishedAt    int64          `json:"finished_at"`
		} `json:"items"`
		Total int `json:"total"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=node_observation", adminToken, &runs)
	if runs.Total == 0 || len(runs.Items) == 0 {
		t.Fatalf("expected at least one maintenance run: %#v", runs)
	}
	var run struct {
		ID            string         `json:"id"`
		RunType       string         `json:"run_type"`
		TriggerSource string         `json:"trigger_source"`
		State         string         `json:"state"`
		Result        string         `json:"result"`
		ReasonCode    string         `json:"reason_code"`
		TotalCount    int            `json:"total_count"`
		FinishedCount int            `json:"finished_count"`
		Detail        map[string]any `json:"detail"`
		CreatedAt     int64          `json:"created_at"`
		FinishedAt    int64          `json:"finished_at"`
	}
	for _, item := range runs.Items {
		if item.TriggerSource == "manual" && item.State == "finished" {
			run = item
			break
		}
	}
	if run.ID == "" {
		t.Fatalf("manual finished node observation run not found: %#v", runs.Items)
	}
	if run.RunType != "node_observation" || run.TriggerSource != "manual" {
		t.Fatalf("run identity = %#v, want manual node_observation", run)
	}
	if run.State != "finished" || run.Result != "success" || run.ReasonCode != "completed" {
		t.Fatalf("run status = %#v, want finished success completed", run)
	}
	if run.TotalCount != 1 || run.FinishedCount != 1 {
		t.Fatalf("run counts = %d/%d, want 1/1", run.FinishedCount, run.TotalCount)
	}
	if run.CreatedAt == 0 || run.FinishedAt == 0 {
		t.Fatalf("run timestamps should be set: %#v", run)
	}
	if run.Detail["target_scope"] != "single_node" {
		t.Fatalf("target_scope = %v, want single_node in %#v", run.Detail["target_scope"], run.Detail)
	}

	var detail struct {
		Run struct {
			ID     string         `json:"id"`
			Detail map[string]any `json:"detail"`
		} `json:"run"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs/"+run.ID, adminToken, &detail)
	if detail.Run.ID != run.ID {
		t.Fatalf("detail id = %q, want %q", detail.Run.ID, run.ID)
	}
	if detail.Run.Detail["success_count"] != float64(1) {
		t.Fatalf("detail success_count = %#v, want 1", detail.Run.Detail["success_count"])
	}

	var overview struct {
		MaintenanceRuns  []any `json:"maintenance_runs"`
		MaintenanceTasks []any `json:"maintenance_tasks"`
	}
	getJSON(t, srv.URL+"/api/overview", adminToken, &overview)
	if len(overview.MaintenanceRuns) == 0 {
		t.Fatalf("overview maintenance_runs should include recent run: %#v", overview)
	}
	if overview.MaintenanceTasks != nil {
		t.Fatalf("overview must not return maintenance_tasks: %#v", overview.MaintenanceTasks)
	}

	resp := get(t, srv.URL+"/api/maintenance/tasks", adminToken)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("old maintenance tasks endpoint status = %d, want 404", resp.StatusCode)
	}
}

func TestFixedProfileManualEvaluationIsRejectedWithoutRun(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "fixed-profile-node",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed profile",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	resp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/actions/evaluate", adminToken, map[string]any{})
	var errorBody struct {
		Error string `json:"error"`
	}
	decodeJSON(t, resp, &errorBody)
	if resp.StatusCode != http.StatusBadRequest || errorBody.Error != "profile_type_not_evaluable" {
		t.Fatalf("evaluate fixed profile status/body = %d %#v, want 400 profile_type_not_evaluable", resp.StatusCode, errorBody)
	}

	var runs struct {
		Items []any `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=profile_evaluation&target_id="+profile.ID, adminToken, &runs)
	if len(runs.Items) != 0 {
		t.Fatalf("fixed profile evaluation should not create runs: %#v", runs.Items)
	}
}

func TestSubscriptionRefreshCreatesMaintenanceRunWithIgnoredAndSkippedSummary(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
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

	createResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "refresh-run-subscription",
		"source_type": "local",
		"content":     content,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	var refreshed struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/subscriptions/"+created.ID+"/refresh", adminToken, map[string]any{}), &refreshed)
	if refreshed.ImportedNodes != 1 || refreshed.SkippedEntries != 2 {
		t.Fatalf("refresh result = %#v, want 1 imported and 2 skipped/ignored entries", refreshed)
	}

	var runs struct {
		Items []struct {
			RunType       string         `json:"run_type"`
			TriggerSource string         `json:"trigger_source"`
			State         string         `json:"state"`
			Result        string         `json:"result"`
			TotalCount    int            `json:"total_count"`
			FinishedCount int            `json:"finished_count"`
			Detail        map[string]any `json:"detail"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=subscription_refresh&target_id="+created.ID, adminToken, &runs)
	if len(runs.Items) != 1 {
		t.Fatalf("subscription refresh runs = %d, want 1: %#v", len(runs.Items), runs.Items)
	}
	run := runs.Items[0]
	if run.RunType != "subscription_refresh" || run.TriggerSource != "manual" || run.State != "finished" || run.Result != "success" {
		t.Fatalf("subscription refresh run identity/status = %#v, want finished manual success", run)
	}
	if run.TotalCount != 1 || run.FinishedCount != 1 {
		t.Fatalf("subscription refresh counts = %d/%d, want 1/1", run.FinishedCount, run.TotalCount)
	}
	if run.Detail["imported_count"] != float64(1) || run.Detail["ignored_count"] != float64(1) || run.Detail["skipped_count"] != float64(1) {
		t.Fatalf("subscription refresh detail = %#v, want imported 1 ignored 1 skipped 1", run.Detail)
	}

	var observationRuns struct {
		Items []struct {
			TriggerSource string `json:"trigger_source"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=node_observation", adminToken, &observationRuns)
	subscriptionTriggered := 0
	for _, item := range observationRuns.Items {
		if item.TriggerSource == "subscription_refresh" {
			subscriptionTriggered++
		}
	}
	if subscriptionTriggered != 1 {
		t.Fatalf("subscription refresh should create one aggregate observation run, got %d in %#v", subscriptionTriggered, observationRuns.Items)
	}
}

func TestSubscriptionRefreshRunWarnsWhenNoNodesImported(t *testing.T) {
	t.Parallel()

	content := `
proxies:
  - name: initial-http
    type: http
    server: 127.0.0.1
    port: 18084
`
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	}))
	t.Cleanup(remote.Close)

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "empty-refresh-subscription",
		"source_type": "remote",
		"url":         remote.URL,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	content = `
proxies:
  - name: missing-server
    type: http
    port: 18085
`

	var refreshed struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/subscriptions/"+created.ID+"/refresh", adminToken, map[string]any{}), &refreshed)
	if refreshed.ImportedNodes != 0 || refreshed.SkippedEntries != 1 {
		t.Fatalf("refresh result = %#v, want 0 imported and 1 skipped", refreshed)
	}

	var runs struct {
		Items []struct {
			RunType       string         `json:"run_type"`
			TriggerSource string         `json:"trigger_source"`
			State         string         `json:"state"`
			Result        string         `json:"result"`
			ReasonCode    string         `json:"reason_code"`
			Detail        map[string]any `json:"detail"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=subscription_refresh&target_id="+created.ID, adminToken, &runs)
	if len(runs.Items) != 1 {
		t.Fatalf("subscription refresh runs = %d, want 1: %#v", len(runs.Items), runs.Items)
	}
	run := runs.Items[0]
	if run.RunType != "subscription_refresh" || run.TriggerSource != "manual" || run.State != "finished" || run.Result != "warning" || run.ReasonCode != "no_importable_nodes" {
		t.Fatalf("subscription refresh run identity/status = %#v, want finished manual warning no_importable_nodes", run)
	}
	if run.Detail["imported_count"] != float64(0) || run.Detail["skipped_count"] != float64(1) {
		t.Fatalf("subscription refresh detail = %#v, want imported 0 skipped 1", run.Detail)
	}
}
