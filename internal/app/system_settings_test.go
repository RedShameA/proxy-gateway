package app_test

import (
	"net/http"
	"net/http/httptest"
	"proxygateway/internal/testsupport/apptest"
	"testing"
	"time"
)

func TestSystemSettingsGet(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	// Setup admin
	postJSON(t, srv.URL+"/api/admin/setup", "", map[string]string{
		"password": "correct horse battery staple",
	})
	var loginResp struct {
		Token string `json:"token"`
	}
	login := postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
		"password": "correct horse battery staple",
	})
	decodeJSON(t, login, &loginResp)
	token := loginResp.Token

	// Get settings
	var settings struct {
		PublicProxyEndpoint                string         `json:"public_proxy_endpoint"`
		LogRetentionEnabled                bool           `json:"log_retention_enabled"`
		LogRetentionDays                   int            `json:"log_retention_days"`
		MaintenanceHistoryRetentionEnabled bool           `json:"maintenance_history_retention_enabled"`
		MaintenanceHistoryRetentionDays    int            `json:"maintenance_history_retention_days"`
		GeoIP                              map[string]any `json:"geoip"`
	}
	getJSON(t, srv.URL+"/api/system/settings", token, &settings)

	// Default values
	if !settings.LogRetentionEnabled {
		t.Error("expected log_retention_enabled=true")
	}
	if settings.LogRetentionDays != 10 {
		t.Errorf("expected log_retention_days=10, got %d", settings.LogRetentionDays)
	}
	if !settings.MaintenanceHistoryRetentionEnabled {
		t.Error("expected maintenance_history_retention_enabled=true")
	}
	if settings.MaintenanceHistoryRetentionDays != 7 {
		t.Errorf("expected maintenance_history_retention_days=7, got %d", settings.MaintenanceHistoryRetentionDays)
	}
}

func TestSystemSettingsGeoIPStatusIncludesSourceAndNextUpdate(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	token := setupAdmin(t, srv.URL)

	var settings struct {
		GeoIP struct {
			Source       string `json:"source"`
			NextUpdateAt *int64 `json:"next_update_at"`
		} `json:"geoip"`
	}
	getJSON(t, srv.URL+"/api/system/settings", token, &settings)

	if settings.GeoIP.Source != "MetaCubeX/meta-rules-dat latest country.mmdb" {
		t.Fatalf("geoip source = %q", settings.GeoIP.Source)
	}
	if settings.GeoIP.NextUpdateAt == nil {
		t.Fatal("geoip next_update_at is nil, want scheduled timestamp")
	}
	if *settings.GeoIP.NextUpdateAt <= time.Now().UnixMilli() {
		t.Fatalf("geoip next_update_at = %d, want future timestamp", *settings.GeoIP.NextUpdateAt)
	}

	resp := patchJSON(t, srv.URL+"/api/system/settings", token, map[string]any{
		"maintenance_schedules": []map[string]any{
			{"key": "geoip_update", "enabled": false},
		},
	})
	decodeOK(t, resp, &settings)
	if settings.GeoIP.NextUpdateAt != nil {
		t.Fatalf("geoip next_update_at = %d, want nil when schedule disabled", *settings.GeoIP.NextUpdateAt)
	}
}

func TestSystemSettingsPatchScanAndEvaluationSettings(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	token := setupAdmin(t, srv.URL)

	var patched struct {
		Maintenance struct {
			NodeObservationConcurrency   int `json:"node_observation_concurrency"`
			ProfileEvaluationConcurrency int `json:"profile_evaluation_concurrency"`
		} `json:"maintenance"`
		Evaluation struct {
			GlobalConcurrency     int `json:"global_concurrency"`
			ConnectTimeoutSeconds int `json:"connect_timeout_seconds"`
			ProbeTimeoutSeconds   int `json:"probe_timeout_seconds"`
		} `json:"evaluation"`
	}
	resp := patchJSON(t, srv.URL+"/api/system/settings", token, map[string]any{
		"maintenance": map[string]any{
			"node_observation_concurrency":   12,
			"profile_evaluation_concurrency": 4,
		},
		"evaluation": map[string]any{
			"global_concurrency":      24,
			"connect_timeout_seconds": 7,
			"probe_timeout_seconds":   9,
		},
	})
	decodeOK(t, resp, &patched)

	if patched.Maintenance.NodeObservationConcurrency != 12 {
		t.Fatalf("node_observation_concurrency = %d, want 12", patched.Maintenance.NodeObservationConcurrency)
	}
	if patched.Maintenance.ProfileEvaluationConcurrency != 4 {
		t.Fatalf("profile_evaluation_concurrency = %d, want 4", patched.Maintenance.ProfileEvaluationConcurrency)
	}
	if patched.Evaluation.GlobalConcurrency != 24 {
		t.Fatalf("global_concurrency = %d, want 24", patched.Evaluation.GlobalConcurrency)
	}
	if patched.Evaluation.ConnectTimeoutSeconds != 7 {
		t.Fatalf("connect_timeout_seconds = %d, want 7", patched.Evaluation.ConnectTimeoutSeconds)
	}
	if patched.Evaluation.ProbeTimeoutSeconds != 9 {
		t.Fatalf("probe_timeout_seconds = %d, want 9", patched.Evaluation.ProbeTimeoutSeconds)
	}
}

func TestSystemSettingsPatchRetentionCleanupSettings(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	token := setupAdmin(t, srv.URL)

	var patched struct {
		LogRetentionEnabled                bool `json:"log_retention_enabled"`
		LogRetentionDays                   int  `json:"log_retention_days"`
		MaintenanceHistoryRetentionEnabled bool `json:"maintenance_history_retention_enabled"`
		MaintenanceHistoryRetentionDays    int  `json:"maintenance_history_retention_days"`
	}
	resp := patchJSON(t, srv.URL+"/api/system/settings", token, map[string]any{
		"log_retention_enabled":                 false,
		"log_retention_days":                    12,
		"maintenance_history_retention_enabled": false,
		"maintenance_history_retention_days":    9,
	})
	decodeOK(t, resp, &patched)

	if patched.LogRetentionEnabled || patched.LogRetentionDays != 12 {
		t.Fatalf("request log retention = %t/%d, want false/12", patched.LogRetentionEnabled, patched.LogRetentionDays)
	}
	if patched.MaintenanceHistoryRetentionEnabled || patched.MaintenanceHistoryRetentionDays != 9 {
		t.Fatalf("maintenance history retention = %t/%d, want false/9", patched.MaintenanceHistoryRetentionEnabled, patched.MaintenanceHistoryRetentionDays)
	}
}

func TestSystemSettingsValidatesPublicProxyEndpoint(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	token := setupAdmin(t, srv.URL)

	var patched struct {
		PublicProxyEndpoint string `json:"public_proxy_endpoint"`
	}
	resp := patchJSON(t, srv.URL+"/api/system/settings", token, map[string]any{
		"public_proxy_endpoint": "  [::1]:8080  ",
	})
	decodeOK(t, resp, &patched)
	if patched.PublicProxyEndpoint != "[::1]:8080" {
		t.Fatalf("public_proxy_endpoint = %q, want trimmed [::1]:8080", patched.PublicProxyEndpoint)
	}

	for _, endpoint := range []string{
		"https://proxy.example.com:8080",
		"proxy.example.com:8080/path",
		"::1",
		"proxy.example.com:99999",
	} {
		resp := patchJSON(t, srv.URL+"/api/system/settings", token, map[string]any{
			"public_proxy_endpoint": endpoint,
		})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("endpoint %q status = %d, want 400", endpoint, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func TestSystemSettingsRequiresAuth(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	resp := get(t, srv.URL+"/api/system/settings", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAdminPasswordChange(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	// Setup admin
	postJSON(t, srv.URL+"/api/admin/setup", "", map[string]string{
		"password": "correct horse battery staple",
	})
	var loginResp struct {
		Token string `json:"token"`
	}
	login := postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
		"password": "correct horse battery staple",
	})
	decodeJSON(t, login, &loginResp)
	token := loginResp.Token

	// Change password
	changeResp := postJSON(t, srv.URL+"/api/admin/password", token, map[string]string{
		"current_password": "correct horse battery staple",
		"new_password":     "new secure password here",
	})
	if changeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", changeResp.StatusCode)
	}

	// Old password should fail
	oldLogin := postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
		"password": "correct horse battery staple",
	})
	if oldLogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 with old password, got %d", oldLogin.StatusCode)
	}

	// New password should work
	newLogin := postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
		"password": "new secure password here",
	})
	if newLogin.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with new password, got %d", newLogin.StatusCode)
	}
}

func TestEgressCountriesEndpoint(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	// Setup admin
	postJSON(t, srv.URL+"/api/admin/setup", "", map[string]string{
		"password": "correct horse battery staple",
	})
	var loginResp struct {
		Token string `json:"token"`
	}
	login := postJSON(t, srv.URL+"/api/admin/login", "", map[string]string{
		"password": "correct horse battery staple",
	})
	decodeJSON(t, login, &loginResp)
	token := loginResp.Token

	// Get egress countries
	var countries []struct {
		Value     string  `json:"value"`
		ISOCode   *string `json:"iso_code"`
		NameZH    string  `json:"name_zh"`
		IsUnknown bool    `json:"is_unknown"`
	}
	getJSON(t, srv.URL+"/api/dictionaries/egress-countries", token, &countries)

	// Should at least have unknown
	if len(countries) == 0 {
		t.Fatal("expected at least one country entry")
	}
	if countries[0].Value != "__unknown__" {
		t.Errorf("expected first entry to be __unknown__, got %s", countries[0].Value)
	}
	if !countries[0].IsUnknown {
		t.Error("expected first entry to be unknown")
	}
}
