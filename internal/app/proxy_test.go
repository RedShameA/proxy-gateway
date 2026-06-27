package app_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"proxygateway/internal/app"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
	"proxygateway/internal/testsupport/apptest"
	"testing"
	"time"
)

func TestManualNodeCanServeFixedNodeHTTPProxyPath(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through fixed node"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	upstreamProxy := newHTTPConnectProxy(t)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	adminToken := setupAdmin(t, srv.URL)

	nodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "upstream-http",
		"type":        "http",
		"server":      upstreamProxy.host,
		"server_port": upstreamProxy.port,
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "fixed.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := client.Get("http://" + targetURL.Host + "/demo")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "through fixed node" {
		t.Fatalf("body = %q", got)
	}
	if upstreamProxy.connects == 0 {
		t.Fatal("gateway did not use the configured HTTP Node")
	}
}

func TestHTTPProxyRejectsInvalidProxyCredential(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "direct",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed direct",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "fixed.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "wrong-password")
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	resp, err := client.Get("http://example.test/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("status = %d, want 407", resp.StatusCode)
	}
}

func TestHTTPProxyUsesProxyAuthorizationNotTargetAuthorization(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "target-user" || password != "target-pass" {
			t.Fatalf("target Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte("target auth preserved"))
	}))
	t.Cleanup(target.Close)

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "direct",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed direct",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "fixed.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	req, err := http.NewRequest(http.MethodGet, target.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("target-user", "target-pass")
	resp, err := httpProxyClient(t, srv.URL, profile.ID, "proxy-password-123").Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "target auth preserved" {
		t.Fatalf("body = %q", body)
	}
}

func TestProxyCredentialLastUsedAtIsThrottled(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(target.Close)

	dataDir := t.TempDir()
	gw, err := app.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "direct",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed direct",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "fixed.client",
		"password": "proxy-password-123",
	})
	var credential struct {
		ID string `json:"id"`
	}
	decodeOK(t, credentialResp, &credential)

	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	recent := time.Now().UnixMilli()
	if _, err := db.Exec(`UPDATE proxy_credentials SET last_used_at = ? WHERE id = ?`, recent, credential.ID); err != nil {
		t.Fatal(err)
	}
	resp, err := httpProxyClient(t, srv.URL, profile.ID, "proxy-password-123").Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	var got int64
	if err := db.QueryRow(`SELECT last_used_at FROM proxy_credentials WHERE id = ?`, credential.ID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != recent {
		t.Fatalf("last_used_at = %d, want throttled value %d", got, recent)
	}

	old := recent - 61_000
	if _, err := db.Exec(`UPDATE proxy_credentials SET last_used_at = ? WHERE id = ?`, old, credential.ID); err != nil {
		t.Fatal(err)
	}
	resp, err = httpProxyClient(t, srv.URL, profile.ID, "proxy-password-123").Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if err := db.QueryRow(`SELECT last_used_at FROM proxy_credentials WHERE id = ?`, credential.ID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got <= old {
		t.Fatalf("last_used_at = %d, want refreshed value > %d", got, old)
	}
}

func TestHTTPProxyRequestLogsFailureStages(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "dead-http",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 1,
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed-dead",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "dead.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	client := httpProxyClient(t, srv.URL, profile.ID, "wrong-password")
	resp, err := client.Get("http://auth-stage.test/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("auth status = %d, want 407", resp.StatusCode)
	}

	client = httpProxyClient(t, srv.URL, "missing-profile", "proxy-password-123")
	resp, err = client.Get("http://profile-stage.test/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("profile status = %d, want 407", resp.StatusCode)
	}

	decodeOK(t, patchJSON(t, srv.URL+"/api/nodes/"+node.ID, adminToken, map[string]any{"enabled": false}), &struct{}{})
	client = httpProxyClient(t, srv.URL, profile.ID, "proxy-password-123")
	resp, err = client.Get("http://path-stage.test/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("path status = %d, want 502", resp.StatusCode)
	}

	decodeOK(t, patchJSON(t, srv.URL+"/api/nodes/"+node.ID, adminToken, map[string]any{"enabled": true}), &struct{}{})
	resp, err = client.Get("http://dial-stage.test/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("dial status = %d, want 502", resp.StatusCode)
	}

	want := map[string]string{
		"auth-stage.test:80":    "authentication",
		"profile-stage.test:80": "profile_selection",
		"path-stage.test:80":    "path_selection",
		"dial-stage.test:80":    "dial",
	}
	for target, stage := range want {
		assertLatestRequestLogStage(t, srv.URL, adminToken, target, stage)
	}
	assertOverviewRecentFailureStage(t, srv.URL, adminToken, "dial-stage.test:80", "dial")
}

func TestHTTPProxyRequestLogRecordsUpstreamFailureStage(t *testing.T) {
	t.Parallel()

	brokenUpstream := newBrokenHTTPConnectProxy(t)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	credentialUser, credentialPassword := createFixedHTTPProxyAccess(t, srv.URL, adminToken, brokenUpstream)

	client := httpProxyClient(t, srv.URL, credentialUser, credentialPassword)
	resp, err := client.Get("http://upstream-stage.test/")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}

	assertLatestRequestLogStage(t, srv.URL, adminToken, "upstream-stage.test:80", "upstream")
}

func TestSOCKS5ConnectUsesExistingAccessProfile(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through socks5"))
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

	upstreamProxy := newHTTPConnectProxy(t)
	gw := apptest.NewGateway(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = gw.Serve(ln) }()

	baseURL := "http://" + ln.Addr().String()
	adminToken := setupAdmin(t, baseURL)

	nodeResp := postJSON(t, baseURL+"/api/nodes", adminToken, map[string]any{
		"name":        "upstream-http",
		"type":        "http",
		"server":      upstreamProxy.host,
		"server_port": upstreamProxy.port,
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)

	profileResp := postJSON(t, baseURL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)

	credentialResp := postJSON(t, baseURL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "fixed.socks",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	conn, err := socks5Connect(ln.Addr().String(), profile.ID, "proxy-password-123", targetHost, targetPort)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("GET /via-socks HTTP/1.1\r\nHost: " + targetURL.Host + "\r\n\r\n"))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "through socks5" {
		t.Fatalf("body = %q", got)
	}
	if upstreamProxy.connects == 0 {
		t.Fatal("gateway did not use the configured Node for SOCKS5")
	}
}

func TestSOCKS5BindAndUDPAssociateReturnFailure(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = gw.Serve(ln) }()

	baseURL := "http://" + ln.Addr().String()
	adminToken := setupAdmin(t, baseURL)
	nodeResp := postJSON(t, baseURL+"/api/nodes", adminToken, map[string]any{
		"name": "direct",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	profileResp := postJSON(t, baseURL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed direct",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	credentialResp := postJSON(t, baseURL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "fixed.socks",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	for _, command := range []byte{0x02, 0x03} {
		code, err := socks5CommandReplyCode(ln.Addr().String(), profile.ID, "proxy-password-123", command, "example.test", 443)
		if err != nil {
			t.Fatalf("command %d: %v", command, err)
		}
		if code == 0x00 {
			t.Fatalf("command %d unexpectedly succeeded", command)
		}
		if code != 0x07 {
			t.Fatalf("command %d reply code = %#x, want 0x07", command, code)
		}
	}
}

func TestProxyRequestLogsMetadataWithoutBodies(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		_, _ = w.Write([]byte("sensitive response body"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	otherTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("other response body"))
	}))
	t.Cleanup(otherTarget.Close)
	otherTargetURL, err := url.Parse(otherTarget.URL)
	if err != nil {
		t.Fatal(err)
	}

	upstreamProxy := newHTTPConnectProxy(t)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	credentialUser, credentialPassword := createFixedHTTPProxyAccess(t, srv.URL, adminToken, upstreamProxy)

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(credentialUser, credentialPassword)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/log-me")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	otherResp, err := client.Get("http://" + otherTargetURL.Host + "/other-log")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, otherResp.Body)
	_ = otherResp.Body.Close()

	filterURL := srv.URL + "/api/request-logs?access_profile_id=" + url.QueryEscape(credentialUser) +
		"&target=" + url.QueryEscape(targetURL.Host) +
		"&success=true"
	requestLogs := waitForRequestLogs(t, filterURL, adminToken, 1)
	logs := struct {
		RequestLogs []map[string]any `json:"request_logs"`
	}{RequestLogs: requestLogs}
	if len(logs.RequestLogs) != 1 {
		t.Fatalf("request log count = %d, want 1", len(logs.RequestLogs))
	}
	log := logs.RequestLogs[0]
	accessProfile, _ := log["access_profile"].(map[string]any)
	proxyCredential, _ := log["proxy_credential"].(map[string]any)
	if accessProfile["id"] != credentialUser || proxyCredential["remark"] != "fixed client" {
		t.Fatalf("unexpected log identity fields: %#v", log)
	}
	if log["target_host"] != targetURL.Host {
		t.Fatalf("target_host = %v, want %s", log["target_host"], targetURL.Host)
	}
	if log["result"] != "success" {
		t.Fatalf("result = %v, want success", log["result"])
	}
	if log["state"] != "completed" || log["success"] != true {
		t.Fatalf("status fields = state %v success %v, want completed true", log["state"], log["success"])
	}
	if duration, ok := log["duration_ms"].(float64); !ok || duration <= 0 {
		t.Fatalf("duration_ms = %v, want positive number", log["duration_ms"])
	}
	if egressBytes, ok := log["egress_bytes"].(float64); !ok || egressBytes <= 0 {
		t.Fatalf("egress_bytes = %v, want positive number", log["egress_bytes"])
	}
	if _, ok := log["request_body"]; ok {
		t.Fatal("log must not expose request_body")
	}
	if _, ok := log["response_body"]; ok {
		t.Fatal("log must not expose response_body")
	}
}

func TestSOCKS5ProxyRequestLogsMetadata(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("socks5 log body must stay out of logs"))
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

	gw := apptest.NewGateway(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = gw.Serve(ln) }()

	baseURL := "http://" + ln.Addr().String()
	adminToken := setupAdmin(t, baseURL)
	nodeResp := postJSON(t, baseURL+"/api/nodes", adminToken, map[string]any{
		"name": "direct-socks-log",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	profileResp := postJSON(t, baseURL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed-socks-log",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, baseURL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "socks-log.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	conn, err := socks5Connect(ln.Addr().String(), profile.ID, "proxy-password-123", targetHost, targetPort)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("GET /socks-log HTTP/1.1\r\nHost: " + targetURL.Host + "\r\nConnection: close\r\n\r\n"))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	_ = conn.Close()

	filterURL := baseURL + "/api/request-logs?proxy_credential=socks-log.client&target=" + url.QueryEscape(targetURL.Host) + "&success=true"
	var logs struct {
		RequestLogs []map[string]any `json:"request_logs"`
	}
	for i := 0; i < 20; i++ {
		getJSON(t, filterURL, adminToken, &logs)
		if len(logs.RequestLogs) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(logs.RequestLogs) != 1 {
		t.Fatalf("SOCKS5 request log count = %d, want 1", len(logs.RequestLogs))
	}
	log := logs.RequestLogs[0]
	accessProfile, _ := log["access_profile"].(map[string]any)
	proxyCredential, _ := log["proxy_credential"].(map[string]any)
	if accessProfile["name"] != "fixed-socks-log" || proxyCredential["remark"] != "socks-log.client" {
		t.Fatalf("unexpected SOCKS5 log identity fields: %#v", log)
	}
	if log["target_host"] != targetURL.Host || log["proxy_path_label"] != "direct-socks-log" {
		t.Fatalf("unexpected SOCKS5 log target/path: %#v", log)
	}
	if log["state"] != "completed" || log["result"] != "success" || log["success"] != true {
		t.Fatalf("unexpected SOCKS5 log status fields: %#v", log)
	}
	if duration, ok := log["duration_ms"].(float64); !ok || duration <= 0 {
		t.Fatalf("duration_ms = %v, want positive number", log["duration_ms"])
	}
	if ingressBytes, ok := log["ingress_bytes"].(float64); !ok || ingressBytes <= 0 {
		t.Fatalf("ingress_bytes = %v, want positive number", log["ingress_bytes"])
	}
	if egressBytes, ok := log["egress_bytes"].(float64); !ok || egressBytes <= 0 {
		t.Fatalf("egress_bytes = %v, want positive number", log["egress_bytes"])
	}
	if _, ok := log["request_body"]; ok {
		t.Fatal("SOCKS5 log must not expose request_body")
	}
	if _, ok := log["response_body"]; ok {
		t.Fatal("SOCKS5 log must not expose response_body")
	}
}

func TestSOCKS5ProxyRequestLogShowsRunningWhileTunnelOpen(t *testing.T) {
	t.Parallel()

	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = targetLn.Close() })
	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		conn, err := targetLn.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- conn
	}()
	targetHost, targetPortText, err := net.SplitHostPort(targetLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var targetPort int
	if _, err := fmt.Sscanf(targetPortText, "%d", &targetPort); err != nil {
		t.Fatal(err)
	}

	gw := apptest.NewGateway(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = gw.Serve(ln) }()

	baseURL := "http://" + ln.Addr().String()
	adminToken := setupAdmin(t, baseURL)
	nodeResp := postJSON(t, baseURL+"/api/nodes", adminToken, map[string]any{
		"name": "direct-running-log",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)
	profileResp := postJSON(t, baseURL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed-running-log",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, baseURL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "running-log.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	conn, err := socks5Connect(ln.Addr().String(), profile.ID, "proxy-password-123", targetHost, targetPort)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
		defer serverConn.Close()
	case err := <-acceptErr:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("target did not accept SOCKS5 tunnel")
	}

	filterURL := baseURL + "/api/request-logs?result=running&target=" + url.QueryEscape(targetLn.Addr().String())
	var logs struct {
		RequestLogs []map[string]any `json:"request_logs"`
	}
	for i := 0; i < 20; i++ {
		getJSON(t, filterURL, adminToken, &logs)
		if len(logs.RequestLogs) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(logs.RequestLogs) != 1 {
		t.Fatalf("running request log count = %d, want 1", len(logs.RequestLogs))
	}
	log := logs.RequestLogs[0]
	if log["state"] != "running" || log["result"] != "running" || log["success"] != nil {
		t.Fatalf("running request log status = %#v, want state/result running and success nil", log)
	}
	if duration, ok := log["duration_ms"].(float64); !ok || duration <= 0 {
		t.Fatalf("running duration_ms = %v, want positive number", log["duration_ms"])
	}

	_ = conn.Close()
	_ = serverConn.Close()
	completedURL := baseURL + "/api/request-logs?result=success&target=" + url.QueryEscape(targetLn.Addr().String())
	for i := 0; i < 20; i++ {
		getJSON(t, completedURL, adminToken, &logs)
		if len(logs.RequestLogs) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(logs.RequestLogs) != 1 {
		t.Fatalf("completed request log count = %d, want 1", len(logs.RequestLogs))
	}
	log = logs.RequestLogs[0]
	if log["state"] != "completed" || log["result"] != "success" || log["success"] != true {
		t.Fatalf("completed request log status = %#v, want completed success", log)
	}
}

func httpProxyClient(t *testing.T, baseURL, username, password string) *http.Client {
	t.Helper()
	proxyURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(username, password)
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
}

func assertLatestRequestLogStage(t *testing.T, baseURL, adminToken, target, wantStage string) {
	t.Helper()
	var logs struct {
		RequestLogs []map[string]any `json:"request_logs"`
	}
	filterURL := baseURL + "/api/request-logs?result=failure&target=" + url.QueryEscape(target)
	for i := 0; i < 20; i++ {
		getJSON(t, filterURL, adminToken, &logs)
		if len(logs.RequestLogs) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(logs.RequestLogs) == 0 {
		t.Fatalf("no failure log for target %s", target)
	}
	if got := logs.RequestLogs[0]["failure_stage"]; got != wantStage {
		t.Fatalf("failure_stage for %s = %v, want %s; log=%#v", target, got, wantStage, logs.RequestLogs[0])
	}
}

func assertOverviewRecentFailureStage(t *testing.T, baseURL, adminToken, target, wantStage string) {
	t.Helper()
	var overview struct {
		RecentFailures []map[string]any `json:"recent_failures"`
	}
	getJSON(t, baseURL+"/api/overview", adminToken, &overview)
	for _, failure := range overview.RecentFailures {
		if failure["target"] == target {
			if got := failure["failure_stage"]; got != wantStage {
				t.Fatalf("overview failure_stage for %s = %v, want %s; failure=%#v", target, got, wantStage, failure)
			}
			return
		}
	}
	t.Fatalf("overview recent failures missing target %s: %#v", target, overview.RecentFailures)
}
