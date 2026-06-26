package app_test

import (
	"bufio"
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

func TestFastestProfileEvaluatesTestURLAndUsesFastestNode(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fastest profile"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	slowProxy := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	fastProxy := newDelayedHTTPConnectProxy(t, 0)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createHTTPNode(t, srv.URL, adminToken, "slow", slowProxy)
	createHTTPNode(t, srv.URL, adminToken, "fast", fastProxy)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	evalResp := postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{})
	var eval struct {
		EvaluatedProfiles int `json:"evaluated_profiles"`
	}
	decodeOK(t, evalResp, &eval)
	if eval.EvaluatedProfiles != 1 {
		t.Fatalf("evaluated_profiles = %d, want 1", eval.EvaluatedProfiles)
	}

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "fastest.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	beforeFastConnects := fastProxy.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/demo")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fastest profile" {
		t.Fatalf("body = %q", body)
	}
	if fastProxy.connects <= beforeFastConnects {
		t.Fatal("request did not use the fastest node")
	}
}

func TestFastestProfileSupportsHTTPSTestURL(t *testing.T) {
	t.Parallel()

	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("https fastest profile"))
	}))
	t.Cleanup(target.Close)

	proxy := newHTTPConnectProxy(t)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeID := createHTTPNode(t, srv.URL, adminToken, "https-node", proxy)
	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "https-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	evalResp := postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{})
	decodeOK(t, evalResp, &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, nodeID, "ready")
}

func TestFastestProfileTreatsUnauthorizedHTTPResponseAsSuccess(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(target.Close)

	proxy := newHTTPConnectProxy(t)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeID := createHTTPNode(t, srv.URL, adminToken, "unauthorized-node", proxy)
	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "unauthorized-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, nodeID, "ready")
}

func TestFastestProfileKeepsCurrentPathUnlessCandidateIsClearlyBetter(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("stable fastest profile"))
	}))
	t.Cleanup(target.Close)

	currentProxy := newDelayedHTTPConnectProxy(t, 0)
	challengerProxy := newDelayedHTTPConnectProxy(t, 120*time.Millisecond)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	settingsResp := postJSON(t, srv.URL+"/api/evaluation-settings", adminToken, map[string]any{
		"global_concurrency":                      4,
		"default_min_evaluation_interval_seconds": 0,
		"single_candidate_limit":                  0,
		"chain_candidate_limit":                   100,
	})
	decodeOK(t, settingsResp, &struct{}{})
	decodeOK(t, patchJSON(t, srv.URL+"/api/system/settings", adminToken, map[string]any{
		"switching_tolerance": map[string]any{
			"relative_improvement_threshold":  0.80,
			"absolute_latency_improvement_ms": 500,
		},
	}), &struct{}{})

	currentNodeID := createHTTPNode(t, srv.URL, adminToken, "stable-current", currentProxy)
	createHTTPNode(t, srv.URL, adminToken, "stable-challenger", challengerProxy)
	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "stable-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	evalResp := postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{})
	decodeOK(t, evalResp, &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, currentNodeID, "ready")

	currentProxy.setDelay(100 * time.Millisecond)
	challengerProxy.setDelay(85 * time.Millisecond)
	evalResp = postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{})
	decodeOK(t, evalResp, &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, currentNodeID, "ready")
}

func TestFastestProfileSwitchesImmediatelyWhenCurrentPathFailsWithUsableCandidate(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fastest failover profile"))
	}))
	t.Cleanup(target.Close)

	currentProxy := newDelayedHTTPConnectProxy(t, 0)
	failoverProxy := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
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

	currentNodeID := createHTTPNode(t, srv.URL, adminToken, "fastest-current-fails", currentProxy)
	failoverNodeID := createHTTPNode(t, srv.URL, adminToken, "fastest-failover", failoverProxy)
	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "fastest-immediate-failover",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, currentNodeID, "ready")

	currentProxy.close()
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, failoverNodeID, "ready")
	assertProfileSwitchReason(t, srv.URL, adminToken, profile.ID, "current_path_failed_switch")
}

func TestFastestProfileEvaluatesCandidatesConcurrentlyAndSelectsAfterAllCandidatesFinish(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("parallel fastest profile"))
	}))
	t.Cleanup(target.Close)

	fastProxy := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	slowProxyA := newDelayedHTTPConnectProxy(t, 600*time.Millisecond)
	slowProxyB := newDelayedHTTPConnectProxy(t, 600*time.Millisecond)
	slowProxyC := newDelayedHTTPConnectProxy(t, 600*time.Millisecond)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	fastNodeID := createHTTPNode(t, srv.URL, adminToken, "parallel-fast", fastProxy)
	createHTTPNode(t, srv.URL, adminToken, "parallel-slow-a", slowProxyA)
	createHTTPNode(t, srv.URL, adminToken, "parallel-slow-b", slowProxyB)
	createHTTPNode(t, srv.URL, adminToken, "parallel-slow-c", slowProxyC)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluation-settings", adminToken, map[string]any{
		"global_concurrency":                      4,
		"default_min_evaluation_interval_seconds": 0,
		"single_candidate_limit":                  0,
		"chain_candidate_limit":                   100,
	}), &struct{}{})

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "parallel-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	started := time.Now()
	done := make(chan struct{})
	go func() {
		defer close(done)
		decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	}()

	waitForProfileState(t, srv.URL, adminToken, profile.ID, 2*time.Second, func(state, currentNodeID string) bool {
		return state == "running" && currentNodeID == ""
	})
	time.Sleep(200 * time.Millisecond)
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, "", "running")
	select {
	case <-done:
		t.Fatal("evaluation finished before all candidates completed")
	default:
	}
	<-done
	elapsed := time.Since(started)
	if elapsed > 1300*time.Millisecond {
		t.Fatalf("evaluation took %s, want candidates evaluated concurrently", elapsed)
	}
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, fastNodeID, "ready")
}

func TestRunningFastestProfileKeepsExistingProxyPathUsable(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("running profile still usable"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	currentProxy := newDelayedHTTPConnectProxy(t, 0)
	slowProxy := newDelayedHTTPConnectProxy(t, 600*time.Millisecond)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	currentNodeID := createHTTPNode(t, srv.URL, adminToken, "running-current", currentProxy)
	createHTTPNode(t, srv.URL, adminToken, "running-slow", slowProxy)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluation-settings", adminToken, map[string]any{
		"global_concurrency":                      2,
		"default_min_evaluation_interval_seconds": 0,
		"single_candidate_limit":                  0,
		"chain_candidate_limit":                   100,
	}), &struct{}{})

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "running-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, currentNodeID, "ready")

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "running.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	}()
	waitForProfileState(t, srv.URL, adminToken, profile.ID, time.Second, func(state, currentNodeID string) bool {
		return state == "running" && currentNodeID != ""
	})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/during-running")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "running profile still usable" {
		t.Fatalf("body = %q", body)
	}
	<-done
}

func TestFastestCountryProfileFiltersByEgressCountry(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("country profile"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	usProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "US")
	jpProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "JP")
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createHTTPNode(t, srv.URL, adminToken, "jp", jpProxy)
	createHTTPNode(t, srv.URL, adminToken, "us", usProxy)
	observeResp := postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": "http://egress.local/",
	})
	decodeOK(t, observeResp, &struct{}{})

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "us-fastest",
		"type":     "fastest",
		"test_url": target.URL,
		"candidate_filter": map[string]any{
			"egress_country_mode": "include",
			"egress_countries":    []string{"US"},
		},
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	evalResp := postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{})
	decodeOK(t, evalResp, &struct{}{})

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "us-fastest.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	beforeUSConnects := usProxy.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/country")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "country profile" {
		t.Fatalf("body = %q", body)
	}
	if usProxy.connects <= beforeUSConnects {
		t.Fatal("request did not use the US node")
	}
}

func TestFastestProfileCountryFilterChangeDropsStaleCurrentPath(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("country filter changed"))
	}))
	t.Cleanup(target.Close)

	hkProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "HK")
	jpProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "JP")
	jpProxy.setDelay(80 * time.Millisecond)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	hkNodeID := createHTTPNode(t, srv.URL, adminToken, "hk-fast", hkProxy)
	jpNodeID := createHTTPNode(t, srv.URL, adminToken, "jp-slow", jpProxy)
	decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": "http://egress.local/",
	}), &struct{}{})

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "filter-change-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, hkNodeID, "ready")

	decodeOK(t, patchJSON(t, srv.URL+"/api/access-profiles/"+profile.ID, adminToken, map[string]any{
		"candidate_filter": map[string]any{
			"egress_country_mode": "include",
			"egress_countries":    []string{"JP"},
		},
	}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, "", "running")

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, jpNodeID, "ready")
}

func TestFastestCountryProfileUsesDefaultTestURLWhenOmitted(t *testing.T) {
	t.Parallel()

	usProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "US")
	jpProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "JP")
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	usNodeID := createHTTPNode(t, srv.URL, adminToken, "default-test-url-us", usProxy)
	createHTTPNode(t, srv.URL, adminToken, "default-test-url-jp", jpProxy)
	observeResp := postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": "http://egress.local/",
	})
	decodeOK(t, observeResp, &struct{}{})

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name": "us-default-test-url",
		"type": "fastest",
		"candidate_filter": map[string]any{
			"egress_country_mode": "include",
			"egress_countries":    []string{"US"},
		},
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	evalResp := postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{})
	decodeOK(t, evalResp, &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, usNodeID, "ready")
}

func TestRandomCountryProfileChoosesUsableNodePerTargetConnection(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("random country"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	usProxyA := newCountryHTTPConnectProxy(t, "egress.local:80", "US")
	usProxyB := newCountryHTTPConnectProxy(t, "egress.local:80", "US")
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createHTTPNode(t, srv.URL, adminToken, "us-a", usProxyA)
	createHTTPNode(t, srv.URL, adminToken, "us-b", usProxyB)
	observeResp := postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": "http://egress.local/",
	})
	decodeOK(t, observeResp, &struct{}{})

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name": "us-random",
		"type": "random",
		"candidate_filter": map[string]any{
			"egress_country_mode": "include",
			"egress_countries":    []string{"US"},
		},
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "us-random.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	beforeA := usProxyA.connects
	beforeB := usProxyB.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	for i := 0; i < 20; i++ {
		resp, err := client.Get("http://" + targetURL.Host + "/random")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if usProxyA.connects <= beforeA || usProxyB.connects <= beforeB {
		t.Fatalf("random profile should use both nodes; before=(%d,%d) after=(%d,%d)", beforeA, beforeB, usProxyA.connects, usProxyB.connects)
	}
}

func TestRandomCountryProfileChoosesUsableNodeForSOCKS5Connect(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("random country socks5"))
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

	usProxyA := newCountryHTTPConnectProxy(t, "egress.local:80", "US")
	usProxyB := newCountryHTTPConnectProxy(t, "egress.local:80", "US")
	gw := app.NewForTest(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = gw.Serve(ln) }()

	baseURL := "http://" + ln.Addr().String()
	adminToken := setupAdmin(t, baseURL)
	createHTTPNode(t, baseURL, adminToken, "us-socks-a", usProxyA)
	createHTTPNode(t, baseURL, adminToken, "us-socks-b", usProxyB)
	decodeOK(t, postJSON(t, baseURL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": "http://egress.local/",
	}), &struct{}{})

	profileResp := postJSON(t, baseURL+"/api/access-profiles", adminToken, map[string]any{
		"name": "us-random-socks",
		"type": "random",
		"candidate_filter": map[string]any{
			"egress_country_mode": "include",
			"egress_countries":    []string{"US"},
		},
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, baseURL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "us-random-socks.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	beforeA := usProxyA.connects
	beforeB := usProxyB.connects
	for i := 0; i < 40; i++ {
		conn, err := socks5Connect(ln.Addr().String(), profile.ID, "proxy-password-123", targetHost, targetPort)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = conn.Write([]byte("GET /random-socks HTTP/1.1\r\nHost: " + targetURL.Host + "\r\n\r\n"))
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			_ = conn.Close()
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		_ = conn.Close()
	}
	if usProxyA.connects <= beforeA || usProxyB.connects <= beforeB {
		t.Fatalf("SOCKS5 random profile should use both nodes; before=(%d,%d) after=(%d,%d)", beforeA, beforeB, usProxyA.connects, usProxyB.connects)
	}
}

func TestAccessProfileCandidateFiltersConstrainEvaluation(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("filtered profile"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	fastProxy := newDelayedHTTPConnectProxy(t, 0)
	slowProxy := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	otherProxy := newDelayedHTTPConnectProxy(t, 0)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	content := `{"outbounds":[
		{"type":"http","tag":"allowed-fast","server":"` + fastProxy.host + `","server_port":` + fastProxy.portText() + `},
		{"type":"http","tag":"blocked-other","server":"` + otherProxy.host + `","server_port":` + otherProxy.portText() + `}
	]}`
	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "filtered-source",
		"source_type": "local",
		"content":     content,
	})
	var sub struct {
		ID string `json:"id"`
	}
	decodeOK(t, subResp, &sub)
	createHTTPNode(t, srv.URL, adminToken, "manual-slow", slowProxy)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":               "filtered-fastest",
		"type":               "fastest",
		"test_url":           target.URL,
		"source_ids":         []string{sub.ID},
		"name_include_regex": "allowed",
		"name_exclude_regex": "blocked",
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	evalResp := postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{})
	decodeOK(t, evalResp, &struct{}{})

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "filtered.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})
	beforeFast := fastProxy.connects
	beforeSlow := slowProxy.connects
	beforeOther := otherProxy.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/filtered")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if fastProxy.connects <= beforeFast {
		t.Fatal("filtered profile did not use allowed-fast")
	}
	if slowProxy.connects != beforeSlow || otherProxy.connects != beforeOther {
		t.Fatalf("filtered profile used excluded candidates: slow %d->%d other %d->%d", beforeSlow, slowProxy.connects, beforeOther, otherProxy.connects)
	}
}

func TestAccessProfileManualOnlyCandidateFilterConstrainEvaluation(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("manual only profile"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	subscriptionProxy := newDelayedHTTPConnectProxy(t, 0)
	manualProxy := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "non-manual-source",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"http","tag":"subscription-fast","server":"` + subscriptionProxy.host + `","server_port":` + subscriptionProxy.portText() + `}]}`,
	})
	decodeOK(t, subResp, &struct{}{})
	manualNodeID := createHTTPNode(t, srv.URL, adminToken, "manual-only-slow", manualProxy)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":        "manual-only-fastest",
		"type":        "fastest",
		"test_url":    target.URL,
		"manual_only": true,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, manualNodeID, "ready")

	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "manual-only.client",
		"password": "proxy-password-123",
	}), &struct{}{})
	beforeManual := manualProxy.connects
	beforeSubscription := subscriptionProxy.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/manual-only")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if manualProxy.connects <= beforeManual {
		t.Fatal("manual_only profile did not use the manual node")
	}
	if subscriptionProxy.connects != beforeSubscription {
		t.Fatalf("manual_only profile used subscription node: %d -> %d", beforeSubscription, subscriptionProxy.connects)
	}
}

func TestAccessProfileNodeSourceModeFiltersCandidates(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("source mode profile"))
	}))
	t.Cleanup(target.Close)

	manualProxy := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	subscriptionProxy := newDelayedHTTPConnectProxy(t, 0)
	otherSubscriptionProxy := newDelayedHTTPConnectProxy(t, 0)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	manualNodeID := createHTTPNode(t, srv.URL, adminToken, "manual-source-mode", manualProxy)
	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "source-mode-sub",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"http","tag":"subscription-source-mode","server":"` + subscriptionProxy.host + `","server_port":` + subscriptionProxy.portText() + `}]}`,
	})
	var sub struct {
		ID string `json:"id"`
	}
	decodeOK(t, subResp, &sub)
	otherSubResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "source-mode-other-sub",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"http","tag":"other-subscription-source-mode","server":"` + otherSubscriptionProxy.host + `","server_port":` + otherSubscriptionProxy.portText() + `}]}`,
	})
	var otherSub struct {
		ID string `json:"id"`
	}
	decodeOK(t, otherSubResp, &otherSub)

	manualProfileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":             "manual-source-mode-profile",
		"type":             "fastest",
		"test_url":         target.URL,
		"node_source_mode": "manual",
	})
	var manualProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, manualProfileResp, &manualProfile)

	subscriptionProfileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":             "subscription-source-mode-profile",
		"type":             "fastest",
		"test_url":         target.URL,
		"node_source_mode": "specific_subscriptions",
		"source_ids":       []string{sub.ID},
	})
	var subscriptionProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, subscriptionProfileResp, &subscriptionProfile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, manualProfile.ID, manualNodeID, "ready")
	assertProfileCurrentNode(t, srv.URL, adminToken, subscriptionProfile.ID, nodeIDByName(t, srv.URL, adminToken, "subscription-source-mode"), "ready")

	var profiles struct {
		AccessProfiles []struct {
			ID             string   `json:"id"`
			NodeSourceMode string   `json:"node_source_mode"`
			SourceIDs      []string `json:"source_ids"`
		} `json:"access_profiles"`
	}
	getJSON(t, srv.URL+"/api/access-profiles", adminToken, &profiles)
	for _, profile := range profiles.AccessProfiles {
		if profile.ID == subscriptionProfile.ID {
			if profile.NodeSourceMode != "specific_subscriptions" || len(profile.SourceIDs) != 1 || profile.SourceIDs[0] != sub.ID {
				t.Fatalf("subscription profile source filter = %#v, want specific sub %s", profile, sub.ID)
			}
			return
		}
	}
	t.Fatalf("profile %s not found: %#v", subscriptionProfile.ID, profiles.AccessProfiles)
}

func TestDisabledNodeIsExcludedFromAutomaticSelection(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("disabled node excluded"))
	}))
	t.Cleanup(target.Close)

	disabledProxy := newDelayedHTTPConnectProxy(t, 0)
	enabledProxy := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
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

	disabledNodeID := createHTTPNode(t, srv.URL, adminToken, "disabled-fast", disabledProxy)
	enabledNodeID := createHTTPNode(t, srv.URL, adminToken, "enabled-slow", enabledProxy)
	decodeOK(t, patchJSON(t, srv.URL+"/api/nodes/"+disabledNodeID, adminToken, map[string]any{
		"enabled": false,
	}), &struct{}{})

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "disabled-excluded-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, enabledNodeID, "ready")
	if disabledProxy.connects != 0 {
		t.Fatalf("disabled node should not be dialed during automatic evaluation; connects=%d", disabledProxy.connects)
	}

	decodeOK(t, patchJSON(t, srv.URL+"/api/nodes/"+disabledNodeID, adminToken, map[string]any{
		"enabled": true,
	}), &struct{}{})
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	if disabledProxy.connects == 0 {
		t.Fatal("re-enabled node should be eligible again")
	}
}

func TestDisabledNodeIsExcludedFromRandomCountrySelection(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("random disabled excluded"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	disabledProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "US")
	enabledProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "US")
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	disabledNodeID := createHTTPNode(t, srv.URL, adminToken, "disabled-random-us", disabledProxy)
	createHTTPNode(t, srv.URL, adminToken, "enabled-random-us", enabledProxy)
	decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": "http://egress.local/",
	}), &struct{}{})
	decodeOK(t, patchJSON(t, srv.URL+"/api/nodes/"+disabledNodeID, adminToken, map[string]any{
		"enabled": false,
	}), &struct{}{})

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name": "random-disabled-excluded",
		"type": "random",
		"candidate_filter": map[string]any{
			"egress_country_mode": "include",
			"egress_countries":    []string{"US"},
		},
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "random-disabled.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	for i := 0; i < 10; i++ {
		resp, err := client.Get("http://" + targetURL.Host + "/random-disabled")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if disabledProxy.connects != 1 {
		t.Fatalf("disabled node should only have observation dial before disable; connects=%d", disabledProxy.connects)
	}
	if enabledProxy.connects <= 1 {
		t.Fatalf("enabled node should serve random requests; connects=%d", enabledProxy.connects)
	}
}

func nodeIDByName(t *testing.T, baseURL, adminToken, name string) string {
	t.Helper()
	var nodes struct {
		Nodes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"nodes"`
	}
	getJSON(t, baseURL+"/api/nodes", adminToken, &nodes)
	for _, node := range nodes.Nodes {
		if node.Name == name {
			return node.ID
		}
	}
	t.Fatalf("node %q not found: %#v", name, nodes.Nodes)
	return ""
}
