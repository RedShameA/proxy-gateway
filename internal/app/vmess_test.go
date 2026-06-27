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
	"proxygateway/internal/testsupport/apptest"
	"testing"

	vmess "github.com/sagernet/sing-vmess"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func TestVMessClashAndURIImportsCreateNodes(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	clashResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "clash-vmess",
		"source_type": "local",
		"content": `
proxies:
  - name: clash-vmess-one
    type: vmess
    server: 127.0.0.1
    port: 19101
    uuid: 7c8dd89a-12bf-4f7e-8a72-8fdd1b6d43d1
    alterId: 0
    cipher: auto
`,
	})
	var clashSub struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, clashResp, &clashSub)
	if clashSub.ImportedNodes != 1 || clashSub.SkippedEntries != 0 {
		t.Fatalf("clash vmess import = %+v, want 1 imported and 0 skipped", clashSub)
	}

	uriPayload := base64.StdEncoding.EncodeToString([]byte(`{
		"v":"2",
		"ps":"uri-vmess-one",
		"add":"127.0.0.1",
		"port":"19102",
		"id":"c18dd5b2-64e9-4c95-9074-df4355f9d554",
		"aid":"0",
		"scy":"auto",
		"net":"tcp",
		"type":"none",
		"host":"",
		"path":"",
		"tls":""
	}`))
	uriResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "uri-vmess",
		"source_type": "local",
		"content":     "vmess://" + uriPayload,
	})
	var uriSub struct {
		ImportedNodes  int `json:"imported_nodes"`
		SkippedEntries int `json:"skipped_entries"`
	}
	decodeOK(t, uriResp, &uriSub)
	if uriSub.ImportedNodes != 1 || uriSub.SkippedEntries != 0 {
		t.Fatalf("uri vmess import = %+v, want 1 imported and 0 skipped", uriSub)
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	found := map[string]bool{}
	for _, node := range nodes.Nodes {
		if node["type"] == "vmess" {
			found[node["name"].(string)] = true
		}
	}
	if !found["clash-vmess-one"] || !found["uri-vmess-one"] {
		t.Fatalf("imported vmess nodes not found: %#v", nodes.Nodes)
	}
}

func TestVMessNormalizedOutboundJSONIncludesProtocolOptionsAndDeduplicatesNodes(t *testing.T) {
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
	userID := "5f162aa9-93f3-4cf3-a6ea-cd0266f4d16b"

	for _, item := range []struct {
		name    string
		content string
	}{
		{
			name: "vmess-a",
			content: `{"outbounds":[{
				"type":"vmess",
				"tag":"vmess-a",
				"server":"127.0.0.1",
				"server_port":19103,
				"uuid":"` + userID + `",
				"security":"auto",
				"alter_id":0,
				"tls":{"enabled":true,"server_name":"example.com"},
				"transport":{"type":"ws","path":"/edge","headers":{"Host":["edge.example.com"]}}
			}]}`,
		},
		{
			name: "vmess-b",
			content: `{"outbounds":[{
				"transport":{"headers":{"Host":["edge.example.com"]},"path":"/edge","type":"ws"},
				"tls":{"server_name":"example.com","enabled":true},
				"security":"auto",
				"uuid":"` + userID + `",
				"server_port":19103,
				"server":"127.0.0.1",
				"tag":"vmess-b",
				"type":"vmess"
			}]}`,
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
		if node["type"] == "vmess" && node["server"] == "127.0.0.1" && node["server_port"].(float64) == 19103 {
			matches++
			nodeID = node["id"].(string)
		}
	}
	if matches != 1 {
		t.Fatalf("deduplicated vmess node matches = %d, want 1; nodes=%#v", matches, nodes.Nodes)
	}
	outboundJSON, _ := readNodeStorage(t, dataDir, nodeID)
	wantOutboundJSON := `{"security":"auto","server":"127.0.0.1","server_port":19103,"tls":{"enabled":true,"server_name":"example.com"},"transport":{"headers":{"Host":["edge.example.com"]},"path":"/edge","type":"ws"},"type":"vmess","uuid":"` + userID + `"}`
	if outboundJSON != wantOutboundJSON {
		t.Fatalf("outbound_json = %s, want %s", outboundJSON, wantOutboundJSON)
	}
}

func TestVMessNodeCanServeFixedNodeHTTPProxyPath(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through vmess"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	userID := "7c8dd89a-12bf-4f7e-8a72-8fdd1b6d43d1"
	vmessHost, vmessPort := newSingVMessServer(t, userID)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "vmess-subscription",
		"source_type": "local",
		"content": fmt.Sprintf(`{"outbounds":[{"type":"vmess","tag":"vmess-one","server":"%s","server_port":%d,"uuid":"%s","security":"auto"}]}`,
			vmessHost,
			vmessPort,
			userID,
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
		if node["name"] == "vmess-one" && node["type"] == "vmess" {
			nodeID = node["id"].(string)
			break
		}
	}
	if nodeID == "" {
		t.Fatalf("imported vmess node not found: %#v", nodes.Nodes)
	}

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "fixed-vmess",
		"type":          "fixed_node",
		"fixed_node_id": nodeID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "vmess.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/via-vmess")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through vmess" {
		t.Fatalf("body = %q", body)
	}
}

func TestVMessMissingRequiredFieldsAreSkipped(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "strict-vmess",
		"source_type": "local",
		"content": `{"outbounds":[
			{"type":"vmess","tag":"ok","server":"127.0.0.1","server_port":19104,"uuid":"7c8dd89a-12bf-4f7e-8a72-8fdd1b6d43d1","security":"auto"},
			{"type":"vmess","tag":"missing-uuid","server":"127.0.0.1","server_port":19105,"security":"auto"},
			{"type":"vmess","tag":"missing-port","server":"127.0.0.1","uuid":"c18dd5b2-64e9-4c95-9074-df4355f9d554","security":"auto"}
		]}`,
	})
	var created struct {
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
	}
	decodeOK(t, resp, &created)
	if created.ImportedNodes != 1 || created.SkippedEntries != 2 {
		t.Fatalf("vmess strict import = %+v, want 1 imported and 2 skipped", created)
	}
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "missing_required_field", 2)
}

func TestVMessNodeObservationRecordsEgressCountry(t *testing.T) {
	t.Parallel()

	egress := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ip":"198.51.100.88","country":"NL"}`))
	}))
	t.Cleanup(egress.Close)

	userID := "0833375f-26e9-4fb9-a18a-d58eea7a0711"
	vmessHost, vmessPort := newSingVMessServer(t, userID)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	nodeID := importVMessNode(t, srv.URL, adminToken, "observe-vmess", vmessHost, vmessPort, userID)

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
		if observation["usable"] != true || observation["egress_country"] != "NL" {
			t.Fatalf("unexpected vmess observation: %#v", observation)
		}
		return
	}
	t.Fatalf("vmess node %s not found: %#v", nodeID, nodes.Nodes)
}

func TestFastestProfileCanSelectVMessNode(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fastest vmess"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	userID := "0b9619e9-6c8e-4cc5-9857-354e9d6479db"
	vmessHost, vmessPort := newSingVMessServer(t, userID)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	nodeID := importVMessNode(t, srv.URL, adminToken, "fastest-vmess", vmessHost, vmessPort, userID)

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "fastest-vmess-profile",
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
		"remark":   "fastest.vmess.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/via-fastest-vmess")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fastest vmess" {
		t.Fatalf("body = %q", body)
	}
}

func TestFastestProfileWithFailingVMessCandidateIsFailedNotNoCandidate(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("unused"))
	}))
	t.Cleanup(target.Close)

	closedPort := freeTCPPort(t)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)
	_ = importVMessNode(t, srv.URL, adminToken, "failing-vmess", "127.0.0.1", closedPort, "6ee56947-7ccd-4cb0-8aaf-5407290f3db1")

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":     "failing-vmess-profile",
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

func importVMessNode(t *testing.T, baseURL, adminToken, name, host string, port int, userID string) string {
	t.Helper()
	subResp := postJSON(t, baseURL+"/api/subscriptions", adminToken, map[string]any{
		"name":        name + "-subscription",
		"source_type": "local",
		"content": fmt.Sprintf(`{"outbounds":[{"type":"vmess","tag":%q,"server":"%s","server_port":%d,"uuid":%q,"security":"auto"}]}`,
			name,
			host,
			port,
			userID,
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
		if node["name"] == name && node["type"] == "vmess" {
			return node["id"].(string)
		}
	}
	t.Fatalf("imported vmess node %q not found: %#v", name, nodes.Nodes)
	return ""
}

func newSingVMessServer(t *testing.T, userID string) (string, int) {
	t.Helper()
	service := vmess.NewService[string](testVMessRelay{})
	if err := service.UpdateUsers([]string{"user"}, []string{userID}, []int{0}); err != nil {
		t.Fatal(err)
	}
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })

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
				done := make(chan error, 1)
				err := service.NewConnection(context.Background(), conn, M.SocksaddrFromNet(conn.RemoteAddr()), func(err error) {
					done <- err
				})
				if err != nil {
					_ = conn.Close()
					return
				}
				<-done
				_ = conn.Close()
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

type testVMessRelay struct{}

func (testVMessRelay) NewConnectionEx(ctx context.Context, conn net.Conn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	go func() {
		if destination.String() == testChainLinkProbeTarget {
			_ = conn.Close()
			onClose(nil)
			return
		}
		upstream, err := net.Dial("tcp", destination.String())
		if err != nil {
			_ = conn.Close()
			onClose(err)
			return
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
		onClose(nil)
	}()
}

func (testVMessRelay) NewPacketConnectionEx(ctx context.Context, conn N.PacketConn, source M.Socksaddr, destination M.Socksaddr, onClose N.CloseHandlerFunc) {
	onClose(nil)
}
