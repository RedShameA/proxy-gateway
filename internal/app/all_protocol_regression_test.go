package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"proxygateway/internal/testsupport/apptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSupportedProtocolObservationsRecordCountryLatencyAndErrors(t *testing.T) {
	t.Parallel()

	egress := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ip":"198.51.100.77","country":"SG"}`))
	}))
	t.Cleanup(egress.Close)

	httpProxy := newHTTPConnectProxy(t)
	socksProxy := newSOCKS5Proxy(t)
	ssHost, ssPort := newSingShadowsocksServer(t, "aes-128-gcm", "all-protocol-observe-ss")
	vmessUserID := "b56fe3b5-9734-4060-a5d9-74ee0d3292f0"
	vmessHost, vmessPort := newSingVMessServer(t, vmessUserID)

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createDirectNode(t, srv.URL, adminToken, "direct-observe")
	createHTTPNode(t, srv.URL, adminToken, "http-observe", httpProxy)
	createSOCKS5Node(t, srv.URL, adminToken, "socks-observe", socksProxy)
	importShadowsocksNode(t, srv.URL, adminToken, "ss-observe-all", ssHost, ssPort, "all-protocol-observe-ss")
	importVMessNode(t, srv.URL, adminToken, "vmess-observe-all", vmessHost, vmessPort, vmessUserID)
	importTrojanNode(t, srv.URL, adminToken, "trojan-observe-unreachable")

	runResp := postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": egress.URL,
	})
	var run struct {
		ObservedNodes int `json:"observed_nodes"`
	}
	decodeOK(t, runResp, &run)
	if run.ObservedNodes != 5 {
		t.Fatalf("observed_nodes = %d, want 5 successful observations", run.ObservedNodes)
	}

	observations := observationsByNodeName(t, srv.URL, adminToken)
	for _, name := range []string{"direct-observe", "http-observe", "socks-observe", "ss-observe-all", "vmess-observe-all"} {
		observation := observations[name]
		if observation["usable"] != true {
			t.Fatalf("%s usable = %v, want true; observation=%#v", name, observation["usable"], observation)
		}
		if observation["egress_country"] != "SG" {
			t.Fatalf("%s egress_country = %v, want SG", name, observation["egress_country"])
		}
		if observation["latency_ms"].(float64) < 0 {
			t.Fatalf("%s latency_ms = %v, want non-negative", name, observation["latency_ms"])
		}
		if observation["last_error"] != "" {
			t.Fatalf("%s last_error = %v, want empty", name, observation["last_error"])
		}
	}

	unreachableObservation := observations["trojan-observe-unreachable"]
	if unreachableObservation["usable"] != false {
		t.Fatalf("unreachable usable = %v, want false", unreachableObservation["usable"])
	}
	if strings.TrimSpace(unreachableObservation["last_error"].(string)) == "" {
		t.Fatalf("unreachable last_error = %v, want connection error", unreachableObservation["last_error"])
	}
}

func TestFastestProfileChoosesFastestNodeAcrossSupportedProtocols(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fastest across protocols"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	slowHTTP := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	fastSOCKS := newDelayedSOCKS5Proxy(t, 0)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createHTTPNode(t, srv.URL, adminToken, "mixed-slow-http", slowHTTP)
	fastNodeID := createSOCKS5Node(t, srv.URL, adminToken, "mixed-fast-socks", fastSOCKS)
	importTrojanNode(t, srv.URL, adminToken, "mixed-unsupported-trojan")

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "mixed-fastest",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, fastNodeID, "ready")

	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "mixed-fastest.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	beforeFastSOCKS := fastSOCKS.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/fastest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fastest across protocols" {
		t.Fatalf("body = %q", body)
	}
	if fastSOCKS.connects <= beforeFastSOCKS {
		t.Fatal("request did not use selected SOCKS5 node")
	}
}

func TestCountryProfilesUseObservedNonHTTPRuntimeProtocols(t *testing.T) {
	t.Parallel()

	egress := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ip":"203.0.113.88","country":"NL"}`))
	}))
	t.Cleanup(egress.Close)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("country non-http"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	ssHost, ssPort := newSingShadowsocksServer(t, "aes-128-gcm", "country-ss-password")
	vmessUserID := "67f80901-2959-4d75-9a28-a69f2991959d"
	vmessHost, vmessPort := newSingVMessServer(t, vmessUserID)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	ssNodeID := importShadowsocksNode(t, srv.URL, adminToken, "country-ss", ssHost, ssPort, "country-ss-password")
	vmessNodeID := importVMessNode(t, srv.URL, adminToken, "country-vmess", vmessHost, vmessPort, vmessUserID)
	decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": egress.URL,
	}), &struct{}{})

	fastestResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "nl-fastest-non-http",
		"type":     "fastest",
		"test_url": target.URL,
		"candidate_filter": map[string]any{
			"egress_country_mode": "include",
			"egress_countries":    []string{"NL"},
		},
	})
	var fastestProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, fastestResp, &fastestProfile)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNodeIn(t, srv.URL, adminToken, fastestProfile.ID, map[string]bool{
		ssNodeID:    true,
		vmessNodeID: true,
	}, "ready")

	randomResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name": "nl-random-non-http",
		"type": "random",
		"candidate_filter": map[string]any{
			"egress_country_mode": "include",
			"egress_countries":    []string{"NL"},
		},
	})
	var randomProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, randomResp, &randomProfile)
	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+randomProfile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "nl-random.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(randomProfile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/random-country")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "country non-http" {
		t.Fatalf("body = %q", body)
	}
}

func TestChainProfilesUseMixedSupportedRuntimeProtocols(t *testing.T) {
	t.Parallel()

	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		_, _ = w.Write([]byte("mixed chain protocols"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	vmessUserID := "0e8794cf-10c3-45fc-a6a9-094ba72373a8"
	vmessHost, vmessPort := newSingVMessServer(t, vmessUserID)
	slowFront := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	fastFront := newDelayedSOCKS5Proxy(t, 0)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	exitNodeID := importVMessNode(t, srv.URL, adminToken, "mixed-vmess-exit", vmessHost, vmessPort, vmessUserID)
	createHTTPNode(t, srv.URL, adminToken, "mixed-slow-http-front", slowFront)
	fastFrontNodeID := createSOCKS5Node(t, srv.URL, adminToken, "mixed-fast-socks-front", fastFront)

	frontProfileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "mixed-fastest-front",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "chain_link",
	})
	var frontProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, frontProfileResp, &frontProfile)
	e2eProfileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "mixed-end-to-end",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "end_to_end",
		"test_url":              target.URL,
	})
	var e2eProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, e2eProfileResp, &e2eProfile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, frontProfile.ID, fastFrontNodeID, "ready")
	assertProfileCurrentNode(t, srv.URL, adminToken, e2eProfile.ID, fastFrontNodeID, "ready")
	if atomic.LoadInt32(&targetHits) == 0 {
		t.Fatal("end-to-end chain evaluation did not fetch Test URL")
	}

	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+e2eProfile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "mixed-chain.client",
		"password": "proxy-password-123",
	}), &struct{}{})
	beforeFastFront := fastFront.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(e2eProfile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/mixed-chain")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "mixed chain protocols" {
		t.Fatalf("body = %q", body)
	}
	if fastFront.connects <= beforeFastFront {
		t.Fatal("request did not use selected SOCKS5 Front Node")
	}
}

func TestProfileFailureStatesAndProxyLogsAcrossProtocolBoundaries(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("body must not be logged"))
	}))
	t.Cleanup(target.Close)

	degradedProxy := newHTTPConnectProxy(t)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluation-settings", adminToken, map[string]any{
		"global_concurrency":                      1,
		"default_min_evaluation_interval_seconds": 0,
		"single_candidate_limit":                  0,
		"chain_candidate_limit":                   100,
	}), &struct{}{})

	degradedNodeID := createHTTPNode(t, srv.URL, adminToken, "degraded-http-node", degradedProxy)
	importTrojanNode(t, srv.URL, adminToken, "failed-trojan-node")

	degradedResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":               "degraded-profile",
		"type":               "fastest",
		"test_url":           target.URL,
		"name_include_regex": "degraded-http-node",
	})
	var degradedProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, degradedResp, &degradedProfile)
	failedResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":               "unsupported-failed-profile",
		"type":               "fastest",
		"test_url":           target.URL,
		"name_include_regex": "failed-trojan-node",
	})
	var failedProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, failedResp, &failedProfile)
	noCandidateResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":               "no-candidate-profile",
		"type":               "fastest",
		"test_url":           target.URL,
		"name_include_regex": "does-not-exist",
	})
	var noCandidateProfile struct {
		ID string `json:"id"`
	}
	decodeOK(t, noCandidateResp, &noCandidateProfile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, degradedProfile.ID, degradedNodeID, "ready")
	assertProfileCurrentNode(t, srv.URL, adminToken, failedProfile.ID, "", "failed")
	assertProfileCurrentNode(t, srv.URL, adminToken, noCandidateProfile.ID, "", "no_candidate")

	degradedProxy.close()
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, degradedProfile.ID, degradedNodeID, "degraded")

	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+degradedProfile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "degraded.client",
		"password": "proxy-password-123",
	}), &struct{}{})
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(degradedProfile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("proxy status = %d, want 502", resp.StatusCode)
	}

	requestLogs := waitForRequestLogs(t, srv.URL+"/api/request-logs", adminToken, 1)
	latest := requestLogs[0]
	if latest["proxy_path_label"] != "degraded-http-node" {
		t.Fatalf("proxy_path_label = %v, want degraded-http-node", latest["proxy_path_label"])
	}
	if latest["result"] != "failure" {
		t.Fatalf("result = %v, want failure", latest["result"])
	}
	if strings.TrimSpace(latest["error"].(string)) == "" {
		t.Fatalf("error summary should be present: %#v", latest)
	}
	logJSON, err := json.Marshal(struct {
		RequestLogs []map[string]any `json:"request_logs"`
	}{RequestLogs: requestLogs})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logJSON), "body must not be logged") {
		t.Fatalf("request logs must not include response bodies: %s", logJSON)
	}
}

func createDirectNode(t *testing.T, baseURL, adminToken, name string) string {
	t.Helper()
	nodeResp := postJSON(t, baseURL+"/api/nodes", adminToken, map[string]any{
		"name": name,
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	return node.ID
}

func createSOCKS5Node(t *testing.T, baseURL, adminToken, name string, upstreamProxy *testSOCKS5Proxy) string {
	t.Helper()
	nodeResp := postJSON(t, baseURL+"/api/nodes", adminToken, map[string]any{
		"name":        name,
		"type":        "socks5",
		"server":      upstreamProxy.host,
		"server_port": upstreamProxy.port,
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	return node.ID
}

func importTrojanNode(t *testing.T, baseURL, adminToken, name string) string {
	t.Helper()
	resp := postJSON(t, baseURL+"/api/subscriptions", adminToken, map[string]any{
		"name":        name + "-subscription",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"trojan","tag":"` + name + `","server":"127.0.0.1","server_port":20401,"password":"secret"}]}`,
	})
	var created struct {
		ImportedNodes int `json:"imported_nodes"`
	}
	decodeOK(t, resp, &created)
	if created.ImportedNodes != 1 {
		t.Fatalf("trojan import = %+v, want one imported node", created)
	}
	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, baseURL+"/api/nodes", adminToken, &nodes)
	for _, node := range nodes.Nodes {
		if node["name"] == name {
			return node["id"].(string)
		}
	}
	t.Fatalf("imported trojan node %q not found: %#v", name, nodes.Nodes)
	return ""
}

func observationsByNodeName(t *testing.T, baseURL, adminToken string) map[string]map[string]any {
	t.Helper()
	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, baseURL+"/api/nodes", adminToken, &nodes)
	observations := map[string]map[string]any{}
	for _, node := range nodes.Nodes {
		name := node["name"].(string)
		observations[name] = node["observation"].(map[string]any)
	}
	return observations
}

func assertProfileCurrentNodeIn(t *testing.T, baseURL, adminToken, profileID string, wantNodeIDs map[string]bool, wantState string) {
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
		if !wantNodeIDs[profile.CurrentNodeID] || profile.State != wantState {
			t.Fatalf("profile %s current_node_id/state = %s/%s, want one of %#v/%s", profileID, profile.CurrentNodeID, profile.State, wantNodeIDs, wantState)
		}
		return
	}
	t.Fatalf("profile %s not found", profileID)
}
