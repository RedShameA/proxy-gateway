package app_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"proxygateway/internal/app"
)

func TestFastestFrontProfileEvaluatesChainLinkAndUsesTwoHopPath(t *testing.T) {
	t.Parallel()

	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		_, _ = w.Write([]byte("through two hop path"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	exitProxy := newHTTPConnectProxy(t)
	exitProxy.allowChainLinkProbeTarget()
	slowFront := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	fastFront := newDelayedHTTPConnectProxy(t, 0)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	exitNodeID := createHTTPNode(t, srv.URL, adminToken, "exit", exitProxy)
	createHTTPNode(t, srv.URL, adminToken, "slow-front", slowFront)
	fastFrontNodeID := createHTTPNode(t, srv.URL, adminToken, "fast-front", fastFront)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "fastest-front",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "chain_link",
		"test_url":              target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	var eval struct {
		EvaluatedProfiles int `json:"evaluated_profiles"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &eval)
	if eval.EvaluatedProfiles != 1 {
		t.Fatalf("evaluated_profiles = %d, want 1", eval.EvaluatedProfiles)
	}
	if atomic.LoadInt32(&targetHits) != 0 {
		t.Fatal("chain_link evaluation must not fetch Test URL")
	}
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, fastFrontNodeID, "ready")

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "chain.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	beforeFastFront := fastFront.connects
	beforeExit := exitProxy.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/two-hop")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through two hop path" {
		t.Fatalf("body = %q", body)
	}
	if fastFront.connects <= beforeFastFront {
		t.Fatal("request did not use selected Front Node")
	}
	if exitProxy.connects <= beforeExit {
		t.Fatal("request did not use Exit Node")
	}

	logs := waitForRequestLogs(t, srv.URL+"/api/request-logs", adminToken, 1)
	if logs[0]["proxy_path_label"] != "fast-front -> exit" {
		t.Fatalf("proxy_path_label = %v, want fast-front -> exit", logs[0]["proxy_path_label"])
	}
}

func TestChainProfileSwitchesImmediatelyWhenCurrentPathFailsWithUsableCandidate(t *testing.T) {
	t.Parallel()

	exitProxy := newHTTPConnectProxy(t)
	exitProxy.allowChainLinkProbeTarget()
	currentFront := newDelayedHTTPConnectProxy(t, 0)
	failoverFront := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
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

	exitNodeID := createHTTPNode(t, srv.URL, adminToken, "chain-failover-exit", exitProxy)
	currentFrontNodeID := createHTTPNode(t, srv.URL, adminToken, "chain-current-fails", currentFront)
	failoverFrontNodeID := createHTTPNode(t, srv.URL, adminToken, "chain-failover-front", failoverFront)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "chain-immediate-failover",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "chain_link",
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, currentFrontNodeID, "ready")

	currentFront.close()
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, failoverFrontNodeID, "ready")
	assertProfileSwitchReason(t, srv.URL, adminToken, profile.ID, "current_path_failed_switch")
}

func TestFastestFrontProfileRequiresExitProtocolHandshake(t *testing.T) {
	t.Parallel()

	fakeExit := newBareTCPServer(t)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	exitNodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "tcp-only-exit",
		"type":        "http",
		"server":      fakeExit.host,
		"server_port": fakeExit.port,
	})
	var exitNode struct {
		ID string `json:"id"`
	}
	decodeOK(t, exitNodeResp, &exitNode)
	directResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "direct-front",
		"type": "direct",
	})
	decodeOK(t, directResp, &struct{}{})

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "requires-protocol",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNode.ID},
		"chain_evaluation_mode": "chain_link",
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})

	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, "", "failed")
}

func TestEndToEndChainProfileEvaluatesFullPathTestURL(t *testing.T) {
	t.Parallel()

	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		_, _ = w.Write([]byte("through end-to-end chain"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	exitProxy := newHTTPConnectProxy(t)
	slowFront := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	fastFront := newDelayedHTTPConnectProxy(t, 0)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	exitNodeID := createHTTPNode(t, srv.URL, adminToken, "e2e-exit", exitProxy)
	createHTTPNode(t, srv.URL, adminToken, "e2e-slow-front", slowFront)
	fastFrontNodeID := createHTTPNode(t, srv.URL, adminToken, "e2e-fast-front", fastFront)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "end-to-end-chain",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "end_to_end",
		"test_url":              target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	var eval struct {
		EvaluatedProfiles int `json:"evaluated_profiles"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &eval)
	if eval.EvaluatedProfiles != 1 {
		t.Fatalf("evaluated_profiles = %d, want 1", eval.EvaluatedProfiles)
	}
	if atomic.LoadInt32(&targetHits) == 0 {
		t.Fatal("end-to-end chain evaluation did not fetch the Test URL")
	}
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, fastFrontNodeID, "ready")

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "e2e-chain.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	beforeFastFront := fastFront.connects
	beforeExit := exitProxy.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/via-e2e")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through end-to-end chain" {
		t.Fatalf("body = %q", body)
	}
	if fastFront.connects <= beforeFastFront {
		t.Fatal("request did not use selected Front Node")
	}
	if exitProxy.connects <= beforeExit {
		t.Fatal("request did not use Exit Node")
	}
}

func TestEndToEndChainProfileTreatsUnauthorizedHTTPResponseAsSuccess(t *testing.T) {
	t.Parallel()

	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(target.Close)

	exitProxy := newHTTPConnectProxy(t)
	frontProxy := newHTTPConnectProxy(t)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	exitNodeID := createHTTPNode(t, srv.URL, adminToken, "unauthorized-exit", exitProxy)
	frontNodeID := createHTTPNode(t, srv.URL, adminToken, "unauthorized-front", frontProxy)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "unauthorized-end-to-end-chain",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "end_to_end",
		"test_url":              target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	if atomic.LoadInt32(&targetHits) == 0 {
		t.Fatal("end-to-end chain evaluation did not fetch the Test URL")
	}
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, frontNodeID, "ready")
}

func TestFastestFrontProfileCanChainHTTPFrontToShadowsocksExit(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through http front to shadowsocks exit"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	ssHost, ssPort := newSingShadowsocksServer(t, "aes-128-gcm", "chain-exit-password")
	frontProxy := newHTTPConnectProxy(t)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	exitNodeID := importShadowsocksNode(t, srv.URL, adminToken, "ss-exit", ssHost, ssPort, "chain-exit-password")
	frontNodeID := createHTTPNode(t, srv.URL, adminToken, "http-front", frontProxy)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "http-to-ss-chain",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "chain_link",
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, frontNodeID, "ready")

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "http-ss-chain.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	beforeFront := frontProxy.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/via-http-ss-chain")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through http front to shadowsocks exit" {
		t.Fatalf("body = %q", body)
	}
	if frontProxy.connects <= beforeFront {
		t.Fatal("chain request did not use HTTP Front Node")
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	if len(nodes.Nodes) != 2 {
		t.Fatalf("runtime chain graph must not create persisted nodes: %#v", nodes.Nodes)
	}
}

func TestEndToEndChainProfileCanEvaluateHTTPFrontToShadowsocksExit(t *testing.T) {
	t.Parallel()

	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		_, _ = w.Write([]byte("end to end http front to shadowsocks exit"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	ssHost, ssPort := newSingShadowsocksServer(t, "aes-128-gcm", "e2e-chain-exit-password")
	slowFront := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	fastFront := newDelayedHTTPConnectProxy(t, 0)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	exitNodeID := importShadowsocksNode(t, srv.URL, adminToken, "e2e-ss-exit", ssHost, ssPort, "e2e-chain-exit-password")
	createHTTPNode(t, srv.URL, adminToken, "e2e-slow-http-front", slowFront)
	fastFrontNodeID := createHTTPNode(t, srv.URL, adminToken, "e2e-fast-http-front", fastFront)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "e2e-http-to-ss-chain",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "end_to_end",
		"test_url":              target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	if atomic.LoadInt32(&targetHits) == 0 {
		t.Fatal("end-to-end chain evaluation did not fetch Test URL through Shadowsocks Exit")
	}
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, fastFrontNodeID, "ready")

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "e2e-http-ss-chain.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	beforeFastFront := fastFront.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/via-e2e-http-ss-chain")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "end to end http front to shadowsocks exit" {
		t.Fatalf("body = %q", body)
	}
	if fastFront.connects <= beforeFastFront {
		t.Fatal("request did not use selected HTTP Front Node")
	}
}

func TestEndToEndChainProfileCanEvaluateHTTPFrontToSOCKS5Exit(t *testing.T) {
	t.Parallel()

	var targetHits int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&targetHits, 1)
		_, _ = w.Write([]byte("end to end http front to socks5 exit"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	exitProxy := newSOCKS5Proxy(t)
	frontProxy := newHTTPConnectProxy(t)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	exitNodeID := createSOCKS5Node(t, srv.URL, adminToken, "e2e-socks5-exit", exitProxy)
	frontNodeID := createHTTPNode(t, srv.URL, adminToken, "e2e-http-front", frontProxy)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "e2e-http-to-socks5-chain",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "end_to_end",
		"test_url":              target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	if atomic.LoadInt32(&targetHits) == 0 {
		t.Fatal("end-to-end chain evaluation did not fetch Test URL through SOCKS5 Exit")
	}
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, frontNodeID, "ready")

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "e2e-http-socks5-chain.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	beforeFront := frontProxy.connects
	beforeExit := exitProxy.connects
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/via-e2e-http-socks5-chain")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "end to end http front to socks5 exit" {
		t.Fatalf("body = %q", body)
	}
	if frontProxy.connects <= beforeFront {
		t.Fatal("request did not use HTTP Front Node")
	}
	if exitProxy.connects <= beforeExit {
		t.Fatal("request did not use SOCKS5 Exit Node")
	}
}

func TestSOCKS5ProxyUsesTwoHopAccessProfilePath(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through socks5 two hop path"))
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

	exitProxy := newHTTPConnectProxy(t)
	exitProxy.allowChainLinkProbeTarget()
	frontProxy := newHTTPConnectProxy(t)
	gw := app.NewForTest(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = gw.Serve(ln) }()

	baseURL := "http://" + ln.Addr().String()
	adminToken := setupAdmin(t, baseURL)
	exitNodeID := createHTTPNode(t, baseURL, adminToken, "socks-exit", exitProxy)
	frontNodeID := createHTTPNode(t, baseURL, adminToken, "socks-front", frontProxy)

	profileResp := postJSON(t, baseURL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "socks-chain",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "chain_link",
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, baseURL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, baseURL, adminToken, profile.ID, frontNodeID, "ready")

	credentialResp := postJSON(t, baseURL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "socks-chain.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	beforeFront := frontProxy.connects
	beforeExit := exitProxy.connects
	conn, err := socks5Connect(ln.Addr().String(), profile.ID, "proxy-password-123", targetHost, targetPort)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("GET /via-socks-chain HTTP/1.1\r\nHost: " + targetURL.Host + "\r\n\r\n"))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through socks5 two hop path" {
		t.Fatalf("body = %q", body)
	}
	if frontProxy.connects <= beforeFront {
		t.Fatal("SOCKS5 request did not use selected Front Node")
	}
	if exitProxy.connects <= beforeExit {
		t.Fatal("SOCKS5 request did not use Exit Node")
	}
}

type bareTCPServer struct {
	host string
	port int
}

func newBareTCPServer(t *testing.T) bareTCPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	host, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return bareTCPServer{host: host, port: port}
}
