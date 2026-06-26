package app_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"proxygateway/internal/app"
)

func TestEvaluationSettingsRoundTrip(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	resp := postJSON(t, srv.URL+"/api/evaluation-settings", adminToken, map[string]any{
		"global_concurrency":                      3,
		"default_min_evaluation_interval_seconds": 90,
		"single_candidate_limit":                  7,
		"chain_candidate_limit":                   11,
	})
	decodeOK(t, resp, &struct{}{})

	var settings struct {
		GlobalConcurrency                   int `json:"global_concurrency"`
		DefaultMinEvaluationIntervalSeconds int `json:"default_min_evaluation_interval_seconds"`
		SingleCandidateLimit                int `json:"single_candidate_limit"`
		ChainCandidateLimit                 int `json:"chain_candidate_limit"`
	}
	getJSON(t, srv.URL+"/api/evaluation-settings", adminToken, &settings)
	if settings.GlobalConcurrency != 3 ||
		settings.DefaultMinEvaluationIntervalSeconds != 90 ||
		settings.SingleCandidateLimit != 7 ||
		settings.ChainCandidateLimit != 11 {
		t.Fatalf("unexpected evaluation settings: %#v", settings)
	}
}

func TestEvaluationBudgetSkipsFreshProfileAndCapsCandidates(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("budgeted profile"))
	}))
	t.Cleanup(target.Close)

	slowProxy := newDelayedHTTPConnectProxy(t, 40*time.Millisecond)
	fastProxy := newDelayedHTTPConnectProxy(t, 0)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	slowNodeID := createHTTPNode(t, srv.URL, adminToken, "slow-first", slowProxy)
	createHTTPNode(t, srv.URL, adminToken, "fast-second", fastProxy)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                            "budgeted-fastest",
		"type":                            "fastest",
		"test_url":                        target.URL,
		"candidate_limit":                 1,
		"min_evaluation_interval_seconds": 3600,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	var firstEval struct {
		EvaluatedProfiles int `json:"evaluated_profiles"`
		SkippedProfiles   int `json:"skipped_profiles"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &firstEval)
	if firstEval.EvaluatedProfiles != 1 || firstEval.SkippedProfiles != 0 {
		t.Fatalf("first evaluation = %#v, want one evaluated profile", firstEval)
	}
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, slowNodeID, "ready")
	if fastProxy.connects != 0 {
		t.Fatalf("candidate limit should avoid testing fast-second; connects=%d", fastProxy.connects)
	}

	var secondEval struct {
		EvaluatedProfiles int `json:"evaluated_profiles"`
		SkippedProfiles   int `json:"skipped_profiles"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &secondEval)
	if secondEval.EvaluatedProfiles != 0 || secondEval.SkippedProfiles != 1 {
		t.Fatalf("second evaluation = %#v, want one skipped fresh profile", secondEval)
	}
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, slowNodeID, "ready")
}

func TestUnavailableProxyPathFailsFastForHTTPAndSOCKS5(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("should not be reached"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	targetHost, targetPortText, err := net.SplitHostPort(targetURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	var targetPort int
	if _, err := fmt.Sscanf(targetPortText, "%d", &targetPort); err != nil {
		t.Fatal(err)
	}

	gw := app.NewForTest(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = gw.Serve(ln) }()

	baseURL := "http://" + ln.Addr().String()
	adminToken := setupAdmin(t, baseURL)
	profileResp := postJSON(t, baseURL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "pending-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	credentialResp := postJSON(t, baseURL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "pending.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	proxyURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/unavailable")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("HTTP proxy status = %d, want 502", resp.StatusCode)
	}

	if err := socks5ConnectExpectFailure(ln.Addr().String(), profile.ID, "proxy-password-123", targetHost, targetPort); err != nil {
		t.Fatal(err)
	}
}

func assertProfileCurrentNode(t *testing.T, baseURL, adminToken, profileID, wantNodeID, wantState string) {
	t.Helper()
	var body struct {
		AccessProfiles []struct {
			ID            string `json:"id"`
			CurrentNodeID string `json:"current_node_id"`
			State         string `json:"state"`
		} `json:"access_profiles"`
	}
	getJSON(t, baseURL+"/api/access-profiles", adminToken, &body)
	for _, profile := range body.AccessProfiles {
		if profile.ID != profileID {
			continue
		}
		if profile.CurrentNodeID != wantNodeID || profile.State != wantState {
			t.Fatalf("profile %s current_node_id/state = %s/%s, want %s/%s", profileID, profile.CurrentNodeID, profile.State, wantNodeID, wantState)
		}
		return
	}
	t.Fatalf("profile %s not found", profileID)
}

func assertProfileSwitchReason(t *testing.T, baseURL, adminToken, profileID, wantReason string) {
	t.Helper()
	var body struct {
		AccessProfiles []struct {
			ID           string `json:"id"`
			SwitchReason string `json:"switch_reason"`
		} `json:"access_profiles"`
	}
	getJSON(t, baseURL+"/api/access-profiles", adminToken, &body)
	for _, profile := range body.AccessProfiles {
		if profile.ID != profileID {
			continue
		}
		if profile.SwitchReason != wantReason {
			t.Fatalf("profile %s switch_reason = %q, want %q", profileID, profile.SwitchReason, wantReason)
		}
		return
	}
	t.Fatalf("profile %s not found", profileID)
}

func waitForProfileState(t *testing.T, baseURL, adminToken, profileID string, timeout time.Duration, match func(state, currentNodeID string) bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var body struct {
			AccessProfiles []struct {
				ID            string `json:"id"`
				CurrentNodeID string `json:"current_node_id"`
				State         string `json:"state"`
			} `json:"access_profiles"`
		}
		getJSON(t, baseURL+"/api/access-profiles", adminToken, &body)
		for _, profile := range body.AccessProfiles {
			if profile.ID == profileID && match(profile.State, profile.CurrentNodeID) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	var body struct {
		AccessProfiles []struct {
			ID            string `json:"id"`
			CurrentNodeID string `json:"current_node_id"`
			State         string `json:"state"`
		} `json:"access_profiles"`
	}
	getJSON(t, baseURL+"/api/access-profiles", adminToken, &body)
	t.Fatalf("profile %s did not reach expected state before timeout; profiles=%#v", profileID, body.AccessProfiles)
}
