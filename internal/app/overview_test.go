package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"proxygateway/internal/app"
)

func TestOverviewEndpoint(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	// Setup admin
	postJSON(t, srv.URL+"/api/admin/setup", "", map[string]string{
		"password": "correct horse battery staple",
	})

	// Get token
	var setupResp struct {
		Token string `json:"token"`
	}
	resp := postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
		"password": "correct horse battery staple",
	})
	decodeJSON(t, resp, &setupResp)
	token := setupResp.Token

	// Call overview
	var overview struct {
		ResourceCounts struct {
			Subscriptions int `json:"subscriptions"`
			Nodes         int `json:"nodes"`
			UsableNodes   int `json:"usable_nodes"`
			Profiles      int `json:"access_profiles"`
			Credentials   int `json:"proxy_credentials"`
			Requests24h   int `json:"requests_24h"`
			Failed24h     int `json:"failed_requests_24h"`
		} `json:"resource_counts"`
		ProfileStateCounts map[string]int `json:"profile_state_counts"`
		RecentFailures     []any          `json:"recent_failures"`
		MaintenanceRuns    []any          `json:"maintenance_runs"`
		MaintenanceTasks   []any          `json:"maintenance_tasks"`
		GeoIPStatus        map[string]any `json:"geoip_status"`
	}
	getJSON(t, srv.URL+"/api/overview", token, &overview)

	// Empty store should have zero counts
	if overview.ResourceCounts.Subscriptions != 0 {
		t.Errorf("expected 0 subscriptions, got %d", overview.ResourceCounts.Subscriptions)
	}
	if overview.ResourceCounts.Nodes != 0 {
		t.Errorf("expected 0 nodes, got %d", overview.ResourceCounts.Nodes)
	}
	if overview.ResourceCounts.Profiles != 0 {
		t.Errorf("expected 0 profiles, got %d", overview.ResourceCounts.Profiles)
	}

	// State counts should exist
	if _, ok := overview.ProfileStateCounts["ready"]; !ok {
		t.Error("profile_state_counts should have 'ready' key")
	}

	// Should not be nil
	if overview.RecentFailures == nil {
		t.Error("recent_failures should not be nil")
	}
	if overview.MaintenanceRuns == nil {
		t.Error("maintenance_runs should not be nil")
	}
	if overview.MaintenanceTasks != nil {
		t.Error("maintenance_tasks should be removed")
	}
}

func TestOverviewRequiresAuth(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/api/overview", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}
