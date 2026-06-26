package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"proxygateway/internal/app"
)

func TestMaintenanceRunsExposeCompletedManualAllNodeObservation(t *testing.T) {
	t.Parallel()

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ip=203.0.113.45\nloc=US\n"))
	}))
	t.Cleanup(probe.Close)

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "observed-maintenance-node",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)

	decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"probe_url": probe.URL,
	}), &struct{}{})

	var body struct {
		Items []struct {
			RunType       string         `json:"run_type"`
			TriggerSource string         `json:"trigger_source"`
			TargetID      string         `json:"target_id"`
			State         string         `json:"state"`
			Result        string         `json:"result"`
			ReasonCode    string         `json:"reason_code"`
			TotalCount    int            `json:"total_count"`
			FinishedCount int            `json:"finished_count"`
			FinishedAt    int64          `json:"finished_at"`
			LastError     string         `json:"last_error"`
			Detail        map[string]any `json:"detail"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=node_observation", adminToken, &body)
	if len(body.Items) == 0 {
		t.Fatal("expected at least one maintenance run")
	}
	var run struct {
		RunType       string         `json:"run_type"`
		TriggerSource string         `json:"trigger_source"`
		TargetID      string         `json:"target_id"`
		State         string         `json:"state"`
		Result        string         `json:"result"`
		ReasonCode    string         `json:"reason_code"`
		TotalCount    int            `json:"total_count"`
		FinishedCount int            `json:"finished_count"`
		FinishedAt    int64          `json:"finished_at"`
		LastError     string         `json:"last_error"`
		Detail        map[string]any `json:"detail"`
	}
	for _, item := range body.Items {
		if item.TriggerSource == "manual" && item.State == "finished" && item.Detail["target_scope"] == "all_nodes" {
			run = item
			break
		}
	}
	if run.RunType == "" {
		t.Fatalf("manual all-node run not found: %#v", body.Items)
	}
	if run.RunType != "node_observation" || run.TriggerSource != "manual" || run.TargetID != "" {
		t.Fatalf("run identity = %#v, want manual all-node observation", run)
	}
	if run.State != "finished" || run.Result != "success" || run.ReasonCode != "completed" {
		t.Fatalf("run status = %#v, want finished success completed", run)
	}
	if run.TotalCount != 1 || run.FinishedCount != 1 {
		t.Fatalf("run counts = %d/%d, want 1/1", run.FinishedCount, run.TotalCount)
	}
	if run.FinishedAt == 0 {
		t.Fatalf("finished_at should be set: %#v", run)
	}
	if run.LastError != "" {
		t.Fatalf("last_error = %q, want empty", run.LastError)
	}
	if run.Detail["target_scope"] != "all_nodes" || run.Detail["success_count"] != float64(1) {
		t.Fatalf("run detail = %#v, want all_nodes success_count 1", run.Detail)
	}
	_ = node.ID
}

func TestSubscriptionAutoRefreshSettingsAreExposedAndPatchable(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":                          "auto-refresh-settings",
		"source_type":                   "local",
		"content":                       `{"outbounds":[{"type":"http","tag":"auto-refresh-node","server":"127.0.0.1","server_port":19080}]}`,
		"auto_refresh_enabled":          false,
		"auto_refresh_interval_seconds": 123,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, subResp, &created)

	var listed struct {
		Subscriptions []struct {
			ID                         string `json:"id"`
			AutoRefreshEnabled         bool   `json:"auto_refresh_enabled"`
			AutoRefreshIntervalSeconds int    `json:"auto_refresh_interval_seconds"`
		} `json:"subscriptions"`
	}
	getJSON(t, srv.URL+"/api/subscriptions", adminToken, &listed)
	if len(listed.Subscriptions) != 1 {
		t.Fatalf("subscriptions = %d, want 1", len(listed.Subscriptions))
	}
	if listed.Subscriptions[0].AutoRefreshEnabled || listed.Subscriptions[0].AutoRefreshIntervalSeconds != 123 {
		t.Fatalf("created auto refresh settings = %#v, want disabled interval 123", listed.Subscriptions[0])
	}

	decodeOK(t, patchJSON(t, srv.URL+"/api/subscriptions/"+created.ID, adminToken, map[string]any{
		"auto_refresh_enabled":          true,
		"auto_refresh_interval_seconds": 456,
	}), &struct{}{})

	getJSON(t, srv.URL+"/api/subscriptions", adminToken, &listed)
	if !listed.Subscriptions[0].AutoRefreshEnabled || listed.Subscriptions[0].AutoRefreshIntervalSeconds != 456 {
		t.Fatalf("patched auto refresh settings = %#v, want enabled interval 456", listed.Subscriptions[0])
	}
}

func TestAccessProfileAutoEvaluationSettingsAreExposedPatchableAndTriggerRun(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                             "auto-eval-settings",
		"type":                             "fastest",
		"test_url":                         "https://example.com/",
		"auto_evaluation_enabled":          false,
		"auto_evaluation_interval_seconds": 111,
		"min_evaluation_interval_seconds":  0,
		"candidate_limit":                  0,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &created)

	var profiles struct {
		AccessProfiles []struct {
			ID                            string `json:"id"`
			ConfigVersion                 int64  `json:"config_version"`
			AutoEvaluationEnabled         bool   `json:"auto_evaluation_enabled"`
			AutoEvaluationIntervalSeconds int    `json:"auto_evaluation_interval_seconds"`
		} `json:"access_profiles"`
	}
	getJSON(t, srv.URL+"/api/access-profiles", adminToken, &profiles)
	if len(profiles.AccessProfiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(profiles.AccessProfiles))
	}
	initialVersion := profiles.AccessProfiles[0].ConfigVersion
	if profiles.AccessProfiles[0].AutoEvaluationEnabled || profiles.AccessProfiles[0].AutoEvaluationIntervalSeconds != 111 {
		t.Fatalf("created auto evaluation settings = %#v, want disabled interval 111", profiles.AccessProfiles[0])
	}

	var initialRuns struct {
		Items []struct {
			TargetID string `json:"target_id"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=profile_evaluation&target_id="+created.ID, adminToken, &initialRuns)
	if len(initialRuns.Items) != 0 {
		t.Fatalf("disabled auto evaluation should not enqueue profile runs: %#v", initialRuns.Items)
	}

	decodeOK(t, patchJSON(t, srv.URL+"/api/access-profiles/"+created.ID, adminToken, map[string]any{
		"auto_evaluation_enabled":          true,
		"auto_evaluation_interval_seconds": 222,
	}), &struct{}{})

	getJSON(t, srv.URL+"/api/access-profiles", adminToken, &profiles)
	if !profiles.AccessProfiles[0].AutoEvaluationEnabled || profiles.AccessProfiles[0].AutoEvaluationIntervalSeconds != 222 {
		t.Fatalf("patched auto evaluation settings = %#v, want enabled interval 222", profiles.AccessProfiles[0])
	}
	if profiles.AccessProfiles[0].ConfigVersion <= initialVersion {
		t.Fatalf("config_version = %d, want greater than %d", profiles.AccessProfiles[0].ConfigVersion, initialVersion)
	}

	var runs struct {
		Items []struct {
			RunType       string         `json:"run_type"`
			TargetID      string         `json:"target_id"`
			TriggerSource string         `json:"trigger_source"`
			State         string         `json:"state"`
			Detail        map[string]any `json:"detail"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=profile_evaluation&target_id="+created.ID, adminToken, &runs)
	if len(runs.Items) != 1 {
		t.Fatalf("profile evaluation runs = %d, want 1: %#v", len(runs.Items), runs.Items)
	}
	run := runs.Items[0]
	if run.RunType != "profile_evaluation" || run.TargetID != created.ID || run.TriggerSource != "access_profile_change" || run.State != "queued" {
		t.Fatalf("queued run = %#v, want access_profile_change queued profile evaluation", run)
	}
	if run.Detail["config_version"] != float64(profiles.AccessProfiles[0].ConfigVersion) {
		t.Fatalf("run config_version = %#v, want %d", run.Detail["config_version"], profiles.AccessProfiles[0].ConfigVersion)
	}
}
