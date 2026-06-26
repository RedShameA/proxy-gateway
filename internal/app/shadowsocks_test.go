package app_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"proxygateway/internal/app"
	"testing"

	"github.com/sagernet/sing-shadowsocks/shadowaead"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func TestShadowsocksClashAndURIImportsCreateNodes(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	clashResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "clash-ss",
		"source_type": "local",
		"content": `
proxies:
  - name: clash-ss-one
    type: ss
    server: 127.0.0.1
    port: 19001
    cipher: aes-128-gcm
    password: clash-secret
`,
	})
	var clashSub struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, clashResp, &clashSub)
	if clashSub.ImportedNodes != 1 || clashSub.SkippedEntries != 0 {
		t.Fatalf("clash ss import = %+v, want 1 imported and 0 skipped", clashSub)
	}

	userInfo := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:uri-secret"))
	uriResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "uri-ss",
		"source_type": "local",
		"content":     "ss://" + userInfo + "@127.0.0.1:19002#uri-ss-one",
	})
	var uriSub struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, uriResp, &uriSub)
	if uriSub.ImportedNodes != 1 || uriSub.SkippedEntries != 0 {
		t.Fatalf("uri ss import = %+v, want 1 imported and 0 skipped", uriSub)
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	found := map[string]bool{}
	for _, node := range nodes.Nodes {
		if node["type"] == "shadowsocks" {
			found[node["name"].(string)] = true
		}
	}
	if !found["clash-ss-one"] || !found["uri-ss-one"] {
		t.Fatalf("imported shadowsocks nodes not found: %#v", nodes.Nodes)
	}
}

func TestShadowsocksMissingRequiredFieldsAreSkipped(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "strict-ss",
		"source_type": "local",
		"content": `{"outbounds":[
			{"type":"shadowsocks","tag":"ok","server":"127.0.0.1","server_port":19003,"method":"aes-128-gcm","password":"ok-secret"},
			{"type":"shadowsocks","tag":"missing-method","server":"127.0.0.1","server_port":19004,"password":"secret"},
			{"type":"shadowsocks","tag":"missing-password","server":"127.0.0.1","server_port":19005,"method":"aes-128-gcm"}
		]}`,
	})
	var created struct {
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
	}
	decodeOK(t, resp, &created)
	if created.ImportedNodes != 1 || created.SkippedEntries != 2 {
		t.Fatalf("shadowsocks strict import = %+v, want 1 imported and 2 skipped", created)
	}
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "missing_required_field", 2)
}

func TestShadowsocksNormalizedOutboundJSONDeduplicatesNodes(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	gw, err := app.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	for _, item := range []struct {
		name    string
		content string
	}{
		{
			name:    "ss-a",
			content: `{"outbounds":[{"type":"shadowsocks","tag":"ss-a","server":"127.0.0.1","server_port":19006,"method":"aes-128-gcm","password":"same-secret"}]}`,
		},
		{
			name:    "ss-b",
			content: `{"outbounds":[{"password":"same-secret","tag":"ss-b","server_port":19006,"server":"127.0.0.1","method":"aes-128-gcm","type":"shadowsocks"}]}`,
		},
	} {
		resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
			"name":        item.name,
			"source_type": "local",
			"content":     item.content,
		})
		decodeOK(t, resp, &struct{}{})
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	matches := 0
	nodeID := ""
	for _, node := range nodes.Nodes {
		if node["type"] == "shadowsocks" && node["server"] == "127.0.0.1" && node["server_port"].(float64) == 19006 {
			matches++
			nodeID = node["id"].(string)
		}
	}
	if matches != 1 {
		t.Fatalf("deduplicated shadowsocks node matches = %d, want 1; nodes=%#v", matches, nodes.Nodes)
	}
	outboundJSON, _ := readNodeStorage(t, dataDir, nodeID)
	wantOutboundJSON := `{"method":"aes-128-gcm","password":"same-secret","server":"127.0.0.1","server_port":19006,"type":"shadowsocks"}`
	if outboundJSON != wantOutboundJSON {
		t.Fatalf("outbound_json = %s, want %s", outboundJSON, wantOutboundJSON)
	}
}

func TestShadowsocksNodeCanServeFixedNodeHTTPProxyPath(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through shadowsocks"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	ssHost, ssPort := newSingShadowsocksServer(t, "aes-128-gcm", "shadowsocks-password")
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "ss-subscription",
		"source_type": "local",
		"content": fmt.Sprintf(`{"outbounds":[{"type":"shadowsocks","tag":"ss-one","server":"%s","server_port":%d,"method":"aes-128-gcm","password":"shadowsocks-password"}]}`,
			ssHost,
			ssPort,
		),
	})
	var sub struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, subResp, &sub)
	if sub.ImportedNodes != 1 || sub.SkippedEntries != 0 {
		t.Fatalf("subscription import = %+v, want 1 imported and 0 skipped", sub)
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	nodeID := ""
	for _, node := range nodes.Nodes {
		if node["name"] == "ss-one" && node["type"] == "shadowsocks" {
			nodeID = node["id"].(string)
			break
		}
	}
	if nodeID == "" {
		t.Fatalf("imported shadowsocks node not found: %#v", nodes.Nodes)
	}

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed-ss",
		"type":          "fixed_node",
		"fixed_node_id": nodeID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "ss.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/via-ss")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through shadowsocks" {
		t.Fatalf("body = %q", body)
	}
}

func TestShadowsocksNodeObservationRecordsEgressCountry(t *testing.T) {
	t.Parallel()

	egress := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ip":"198.51.100.44","country":"SG"}`))
	}))
	t.Cleanup(egress.Close)

	ssHost, ssPort := newSingShadowsocksServer(t, "aes-128-gcm", "observe-password")
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	nodeID := importShadowsocksNode(t, srv.URL, adminToken, "observe-ss", ssHost, ssPort, "observe-password")

	runResp := postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": egress.URL,
	})
	var run struct {
		ObservedNodes int `json:"observed_nodes"`
	}
	decodeOK(t, runResp, &run)
	if run.ObservedNodes != 1 {
		t.Fatalf("observed_nodes = %d, want 1", run.ObservedNodes)
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	for _, node := range nodes.Nodes {
		if node["id"] != nodeID {
			continue
		}
		observation := node["observation"].(map[string]any)
		if observation["usable"] != true || observation["egress_country"] != "SG" {
			t.Fatalf("unexpected shadowsocks observation: %#v", observation)
		}
		return
	}
	t.Fatalf("shadowsocks node %s not found: %#v", nodeID, nodes.Nodes)
}

func TestFastestProfileCanSelectShadowsocksNode(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fastest shadowsocks"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	ssHost, ssPort := newSingShadowsocksServer(t, "aes-128-gcm", "fastest-password")
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	nodeID := importShadowsocksNode(t, srv.URL, adminToken, "fastest-ss", ssHost, ssPort, "fastest-password")

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "fastest-ss-profile",
		"type":     "fastest",
		"test_url": target.URL,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	evalResp := postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{})
	decodeOK(t, evalResp, &struct{}{})

	var profiles struct {
		AccessProfiles []map[string]any `json:"access_profiles"`
	}
	getJSON(t, srv.URL+"/api/access-profiles", adminToken, &profiles)
	if len(profiles.AccessProfiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(profiles.AccessProfiles))
	}
	if profiles.AccessProfiles[0]["state"] != "ready" || profiles.AccessProfiles[0]["current_node_id"] != nodeID {
		t.Fatalf("unexpected fastest profile state: %#v", profiles.AccessProfiles[0])
	}

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "fastest.ss.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/via-fastest-ss")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fastest shadowsocks" {
		t.Fatalf("body = %q", body)
	}
}

func TestFastestProfileWithFailingShadowsocksCandidateIsFailedNotNoCandidate(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("unused"))
	}))
	t.Cleanup(target.Close)

	closedPort := freeTCPPort(t)
	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	_ = importShadowsocksNode(t, srv.URL, adminToken, "failing-ss", "127.0.0.1", closedPort, "failing-password")

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "failing-ss-profile",
		"type":     "fastest",
		"test_url": target.URL,
	})
	decodeOK(t, profileResp, &struct{}{})
	evalResp := postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{})
	decodeOK(t, evalResp, &struct{}{})

	var profiles struct {
		AccessProfiles []map[string]any `json:"access_profiles"`
	}
	getJSON(t, srv.URL+"/api/access-profiles", adminToken, &profiles)
	if len(profiles.AccessProfiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(profiles.AccessProfiles))
	}
	if profiles.AccessProfiles[0]["state"] != "failed" {
		t.Fatalf("state = %v, want failed; profile=%#v", profiles.AccessProfiles[0]["state"], profiles.AccessProfiles[0])
	}
	if profiles.AccessProfiles[0]["state"] == "no_candidate" {
		t.Fatalf("candidate dial failure must not be no_candidate: %#v", profiles.AccessProfiles[0])
	}
}

func importShadowsocksNode(t *testing.T, baseURL, adminToken, name, host string, port int, password string) string {
	t.Helper()
	subResp := postJSON(t, baseURL+"/api/subscriptions", adminToken, map[string]any{
		"name":        name + "-subscription",
		"source_type": "local",
		"content": fmt.Sprintf(`{"outbounds":[{"type":"shadowsocks","tag":%q,"server":"%s","server_port":%d,"method":"aes-128-gcm","password":%q}]}`,
			name,
			host,
			port,
			password,
		),
	})
	var sub struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, subResp, &sub)
	if sub.ImportedNodes != 1 || sub.SkippedEntries != 0 {
		t.Fatalf("subscription import = %+v, want 1 imported and 0 skipped", sub)
	}
	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, baseURL+"/api/nodes", adminToken, &nodes)
	for _, node := range nodes.Nodes {
		if node["name"] == name && node["type"] == "shadowsocks" {
			return node["id"].(string)
		}
	}
	t.Fatalf("imported shadowsocks node %q not found: %#v", name, nodes.Nodes)
	return ""
}

func newSingShadowsocksServer(t *testing.T, method, password string) (string, int) {
	t.Helper()
	service, err := shadowaead.NewService(method, nil, password, 60, testShadowsocksRelay{})
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_ = service.NewConnection(context.Background(), conn, M.Metadata{Source: M.SocksaddrFromNet(conn.RemoteAddr())})
			}()
		}
	}()
	host, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	return host, port
}

type testShadowsocksRelay struct{}

func (testShadowsocksRelay) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	if metadata.Destination.String() == testChainLinkProbeTarget {
		_ = conn.Close()
		return nil
	}
	upstream, err := net.Dial("tcp", metadata.Destination.String())
	if err != nil {
		return err
	}
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, conn)
		_ = upstream.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(conn, upstream)
		_ = conn.Close()
		done <- struct{}{}
	}()
	<-done
	return nil
}

func (testShadowsocksRelay) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	return nil
}

func (testShadowsocksRelay) NewError(ctx context.Context, err error) {}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	return port
}
