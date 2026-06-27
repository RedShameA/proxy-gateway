package app_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"net/http/httptest"
	"proxygateway/internal/app"
	singboxinfra "proxygateway/internal/infrastructure/singbox"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
	"proxygateway/internal/testsupport/apptest"
)

func TestRemainingSingBoxProtocolsImportAndNormalize(t *testing.T) {
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

	content := `{"outbounds":[
		{"type":"trojan","tag":"trojan-one","server":"127.0.0.1","server_port":20001,"password":"secret","tls":{"enabled":true,"server_name":"trojan.example"}},
		{"type":"trojan","tag":"trojan-missing-password","server":"127.0.0.1","server_port":20001},
		{"type":"naive","tag":"naive-one","server":"127.0.0.1","server_port":20002,"username":"user","password":"secret","tls":{"enabled":true,"server_name":"naive.example"}},
		{"type":"naive","tag":"naive-missing-user","server":"127.0.0.1","server_port":20002,"password":"secret","tls":{"enabled":true}},
		{"type":"wireguard","tag":"wireguard-one","address":["10.0.0.2/32"],"private_key":"private-key","peers":[{"address":"127.0.0.1","port":20003,"public_key":"public-key","allowed_ips":["0.0.0.0/0"]}]},
		{"type":"wireguard","tag":"wireguard-missing-private-key","address":["10.0.0.2/32"],"peers":[{"address":"127.0.0.1","port":20003,"public_key":"public-key"}]},
		{"type":"hysteria","tag":"hysteria-one","server":"127.0.0.1","server_port":20004,"auth_str":"secret","up_mbps":10,"down_mbps":10,"tls":{"enabled":true,"server_name":"hysteria.example"}},
		{"type":"hysteria","tag":"hysteria-missing-auth","server":"127.0.0.1","server_port":20004,"tls":{"enabled":true}},
		{"type":"shadowtls","tag":"shadowtls-one","server":"127.0.0.1","server_port":20005,"version":3,"password":"secret","tls":{"enabled":true,"server_name":"shadowtls.example"}},
		{"type":"shadowtls","tag":"shadowtls-missing-password","server":"127.0.0.1","server_port":20005},
		{"type":"vless","tag":"vless-one","server":"127.0.0.1","server_port":20006,"uuid":"7c8dd89a-12bf-4f7e-8a72-8fdd1b6d43d1","tls":{"enabled":true,"server_name":"vless.example"}},
		{"type":"vless","tag":"vless-missing-uuid","server":"127.0.0.1","server_port":20006},
		{"type":"tuic","tag":"tuic-one","server":"127.0.0.1","server_port":20007,"uuid":"7c8dd89a-12bf-4f7e-8a72-8fdd1b6d43d1","password":"secret","tls":{"enabled":true,"server_name":"tuic.example"}},
		{"type":"tuic","tag":"tuic-missing-password","server":"127.0.0.1","server_port":20007,"uuid":"7c8dd89a-12bf-4f7e-8a72-8fdd1b6d43d1"},
		{"type":"hysteria2","tag":"hysteria2-one","server":"127.0.0.1","server_port":20008,"password":"secret","tls":{"enabled":true,"server_name":"hysteria2.example"}},
		{"type":"hysteria2","tag":"hysteria2-missing-password","server":"127.0.0.1","server_port":20008},
		{"type":"anytls","tag":"anytls-one","server":"127.0.0.1","server_port":20009,"password":"secret","tls":{"enabled":true,"server_name":"anytls.example"}},
		{"type":"anytls","tag":"anytls-missing-password","server":"127.0.0.1","server_port":20009},
		{"type":"tor","tag":"tor-one","executable_path":"tor"},
		{"type":"tor","tag":"tor-detour-unsupported","detour":"other"},
		{"type":"ssh","tag":"ssh-one","server":"127.0.0.1","server_port":20010,"user":"root","password":"secret"},
		{"type":"ssh","tag":"ssh-missing-user","server":"127.0.0.1","server_port":20010,"password":"secret"}
	]}`
	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "remaining-protocols",
		"source_type": "local",
		"content":     content,
	})
	var created struct {
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
	}
	decodeOK(t, resp, &created)
	if created.ImportedNodes != 11 {
		t.Fatalf("imported_nodes = %d, want 11", created.ImportedNodes)
	}
	if created.SkippedEntries != 11 {
		t.Fatalf("skipped_entries = %d, want 11", created.SkippedEntries)
	}
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "missing_required_field", 10)
	assertSkippedReasonCount(t, created.SkippedEntrySummary, "unsupported_option", 1)

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes?page_size=20", adminToken, &nodes)
	wantTypes := map[string]string{
		"trojan-one":    "trojan",
		"naive-one":     "naive",
		"wireguard-one": "wireguard",
		"hysteria-one":  "hysteria",
		"shadowtls-one": "shadowtls",
		"vless-one":     "vless",
		"tuic-one":      "tuic",
		"hysteria2-one": "hysteria2",
		"anytls-one":    "anytls",
		"tor-one":       "tor",
		"ssh-one":       "ssh",
	}
	for _, node := range nodes.Nodes {
		name := node["name"].(string)
		wantType, ok := wantTypes[name]
		if !ok {
			continue
		}
		if node["type"] != wantType {
			t.Fatalf("%s type = %v, want %s", name, node["type"], wantType)
		}
		sources := node["sources"].([]any)
		if len(sources) != 1 {
			t.Fatalf("%s sources = %#v, want one subscription source", name, sources)
		}
		outboundJSON := readNodeOutboundJSONByName(t, dataDir, name)
		if strings.Contains(outboundJSON, `"tag"`) {
			t.Fatalf("%s outbound_json should not persist tag: %s", name, outboundJSON)
		}
		if !strings.Contains(outboundJSON, `"type":"`+wantType+`"`) {
			t.Fatalf("%s outbound_json = %s, want type %s", name, outboundJSON, wantType)
		}
		delete(wantTypes, name)
	}
	if len(wantTypes) != 0 {
		t.Fatalf("missing imported protocol nodes: %#v", wantTypes)
	}
}

func TestRemainingProtocolsImportFromClashAndURILines(t *testing.T) {
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
	userID := "7c8dd89a-12bf-4f7e-8a72-8fdd1b6d43d1"

	clashYAML := `
proxies:
  - name: clash-trojan
    type: trojan
    server: 127.0.0.1
    port: 20101
    password: trojan-secret
    sni: trojan.example
  - name: clash-vless
    type: vless
    server: 127.0.0.1
    port: 20102
    uuid: ` + userID + `
    tls: true
    servername: vless.example
  - name: clash-hysteria2
    type: hysteria2
    server: 127.0.0.1
    port: 20103
    password: hy2-secret
  - name: clash-tuic
    type: tuic
    server: 127.0.0.1
    port: 20104
    uuid: ` + userID + `
    password: tuic-secret
  - name: clash-vless-missing-uuid
    type: vless
    server: 127.0.0.1
    port: 20105
`
	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "remaining-clash",
		"source_type": "local",
		"content":     clashYAML,
	})
	var clashCreated struct {
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
	}
	decodeOK(t, resp, &clashCreated)
	if clashCreated.ImportedNodes != 4 || clashCreated.SkippedEntries != 1 {
		t.Fatalf("clash import = %+v, want 4 imported and 1 skipped", clashCreated)
	}
	assertSkippedReasonCount(t, clashCreated.SkippedEntrySummary, "missing_required_field", 1)

	uriLines := strings.Join([]string{
		"trojan://trojan-secret@127.0.0.1:20111?sni=trojan.example#uri-trojan",
		"vless://" + userID + "@127.0.0.1:20112?security=tls&sni=vless.example&type=ws&host=edge.example&path=%2Fedge#uri-vless",
		"vless://" + userID + "@cinder.fun:25956?encryption=none&fp=chrome&pbk=l8Cxu3wqhwRobUjH0IYSKwvNqtm0TvF9YH1pdLypyi4&security=reality&sid=03e8026533480a9e&sni=www.amazon.com&spx=%2F0nl8p7z9jecCrGj&type=tcp#uri-vless-reality",
		"hysteria2://hy2-secret@127.0.0.1:20113?sni=hy2.example#uri-hysteria2",
		"hy2://hy2-secret@127.0.0.1:20114#uri-hy2",
		"vless://127.0.0.1:20115#bad-vless",
	}, "\n")
	resp = postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "remaining-uri",
		"source_type": "local",
		"content":     uriLines,
	})
	var uriCreated struct {
		ImportedNodes       int                      `json:"imported_nodes"`
		SkippedEntries      int                      `json:"skipped_entries"`
		SkippedEntrySummary []map[string]interface{} `json:"skipped_entry_summary"`
	}
	decodeOK(t, resp, &uriCreated)
	if uriCreated.ImportedNodes != 5 || uriCreated.SkippedEntries != 1 {
		t.Fatalf("uri import = %+v, want 5 imported and 1 skipped", uriCreated)
	}
	assertSkippedReasonCount(t, uriCreated.SkippedEntrySummary, "missing_required_field", 1)

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	wantTypes := map[string]string{
		"clash-trojan":      "trojan",
		"clash-vless":       "vless",
		"clash-hysteria2":   "hysteria2",
		"clash-tuic":        "tuic",
		"uri-trojan":        "trojan",
		"uri-vless":         "vless",
		"uri-vless-reality": "vless",
		"uri-hysteria2":     "hysteria2",
		"uri-hy2":           "hysteria2",
	}
	for _, node := range nodes.Nodes {
		name := node["name"].(string)
		wantType, ok := wantTypes[name]
		if !ok {
			continue
		}
		if node["type"] != wantType {
			t.Fatalf("%s type = %v, want %s", name, node["type"], wantType)
		}
		outboundJSON := readNodeOutboundJSONByName(t, dataDir, name)
		if !strings.Contains(outboundJSON, `"type":"`+wantType+`"`) {
			t.Fatalf("%s outbound_json = %s, want type %s", name, outboundJSON, wantType)
		}
		if name == "uri-vless-reality" {
			for _, want := range []string{`"reality":{"enabled":true,"public_key":"l8Cxu3wqhwRobUjH0IYSKwvNqtm0TvF9YH1pdLypyi4","short_id":"03e8026533480a9e"}`, `"utls":{"enabled":true,"fingerprint":"chrome"}`, `"server_name":"www.amazon.com"`} {
				if !strings.Contains(outboundJSON, want) {
					t.Fatalf("%s outbound_json = %s, missing %s", name, outboundJSON, want)
				}
			}
			assertRuntimeOutboundParses(t, outboundJSON)
		}
		delete(wantTypes, name)
	}
	if len(wantTypes) != 0 {
		t.Fatalf("missing imported clash/URI nodes: %#v", wantTypes)
	}
}

func TestResinStyleFormatsImportForSubscriptionsAndManualNodes(t *testing.T) {
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

	surge := `
[Proxy]
surge-ss = ss, 127.0.0.1, 20401, encrypt-method=aes-128-gcm, password=secret, obfs=http, obfs-host=cdn.example
surge-vless = vless, 127.0.0.1, 20402, username=7c8dd89a-12bf-4f7e-8a72-8fdd1b6d43d1, tls=true, sni=vless.example, ws=true, ws-path=/edge, host=edge.example
surge-hy2 = hy2, 127.0.0.1, 20403, password=hy2-secret, sni=hy2.example, ports=20403-20405, obfs=salamander, obfs-password=obfs-secret
`
	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "resin-style-surge",
		"source_type": "local",
		"content":     surge,
	})
	var created struct {
		ImportedNodes int `json:"imported_nodes"`
	}
	decodeOK(t, resp, &created)
	if created.ImportedNodes != 3 {
		t.Fatalf("surge imported_nodes = %d, want 3", created.ImportedNodes)
	}

	ssOutbound := readNodeOutboundJSONByName(t, dataDir, "surge-ss")
	if !strings.Contains(ssOutbound, `"plugin":"obfs-local"`) || !strings.Contains(ssOutbound, `"plugin_opts":"obfs=http;obfs-host=cdn.example"`) {
		t.Fatalf("surge ss outbound_json = %s, want obfs plugin", ssOutbound)
	}
	vlessOutbound := readNodeOutboundJSONByName(t, dataDir, "surge-vless")
	for _, want := range []string{`"type":"vless"`, `"server_name":"vless.example"`, `"type":"ws"`, `"path":"/edge"`, `"Host":"edge.example"`} {
		if !strings.Contains(vlessOutbound, want) {
			t.Fatalf("surge vless outbound_json = %s, missing %s", vlessOutbound, want)
		}
	}
	hy2Outbound := readNodeOutboundJSONByName(t, dataDir, "surge-hy2")
	for _, want := range []string{`"type":"hysteria2"`, `"server_ports":["20403:20405"]`, `"obfs":{"password":"obfs-secret","type":"salamander"}`} {
		if !strings.Contains(hy2Outbound, want) {
			t.Fatalf("surge hy2 outbound_json = %s, missing %s", hy2Outbound, want)
		}
	}

	manualResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"import_text": "127.0.0.1:20404:plain-user:plain-pass",
	})
	var manual struct {
		ImportedNodes int `json:"imported_nodes"`
	}
	decodeOK(t, manualResp, &manual)
	if manual.ImportedNodes != 1 {
		t.Fatalf("manual plain proxy imported_nodes = %d, want 1", manual.ImportedNodes)
	}
	plainOutbound := readNodeOutboundJSONByName(t, dataDir, "127.0.0.1:20404")
	for _, want := range []string{`"type":"http"`, `"username":"plain-user"`, `"password":"plain-pass"`} {
		if !strings.Contains(plainOutbound, want) {
			t.Fatalf("manual plain outbound_json = %s, missing %s", plainOutbound, want)
		}
	}
}

func TestRemainingProtocolOutboundJSONDeduplicatesEquivalentSources(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	for _, item := range []struct {
		name    string
		content string
	}{
		{
			name:    "trojan-a",
			content: `{"outbounds":[{"type":"trojan","tag":"trojan-a","server":"127.0.0.1","server_port":20201,"password":"same-secret","tls":{"enabled":true,"server_name":"trojan.example"}}]}`,
		},
		{
			name:    "trojan-b",
			content: `{"outbounds":[{"tls":{"server_name":"trojan.example","enabled":true},"password":"same-secret","server_port":20201,"tag":"trojan-b","server":"127.0.0.1","type":"trojan"}]}`,
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
	var sources []any
	for _, node := range nodes.Nodes {
		if node["type"] == "trojan" && node["server"] == "127.0.0.1" && node["server_port"].(float64) == 20201 {
			matches++
			sources = node["sources"].([]any)
		}
	}
	if matches != 1 {
		t.Fatalf("deduplicated trojan node matches = %d, want 1; nodes=%#v", matches, nodes.Nodes)
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %d, want 2: %#v", len(sources), sources)
	}
}

func TestRemainingProtocolFixedProfileRecordsDialFailure(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("should not be reached"))
	}))
	t.Cleanup(target.Close)

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "unreachable-runtime",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"trojan","tag":"trojan-runtime","server":"127.0.0.1","server_port":20301,"password":"secret"}]}`,
	})
	decodeOK(t, resp, &struct{}{})

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	nodeID := ""
	for _, node := range nodes.Nodes {
		if node["name"] == "trojan-runtime" {
			nodeID = node["id"].(string)
			break
		}
	}
	if nodeID == "" {
		t.Fatalf("trojan-runtime node not found: %#v", nodes.Nodes)
	}

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "unsupported-runtime-fixed",
		"type":          "fixed_node",
		"fixed_node_id": nodeID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "unsupported-runtime.client",
		"password": "proxy-password-123",
	}), &struct{}{})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	proxyResp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, proxyResp.Body)
	_ = proxyResp.Body.Close()
	if proxyResp.StatusCode != http.StatusBadGateway {
		t.Fatalf("proxy status = %d, want 502", proxyResp.StatusCode)
	}

	requestLogs := waitForRequestLogs(t, srv.URL+"/api/request-logs", adminToken, 1)
	if strings.TrimSpace(requestLogs[0]["error"].(string)) == "" {
		t.Fatalf("log error = %v, want dial error", requestLogs[0]["error"])
	}
}

func readNodeOutboundJSONByName(t *testing.T, dataDir, name string) string {
	t.Helper()
	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var outboundJSON string
	if err := db.QueryRow(`SELECT outbound_json FROM nodes WHERE name = ?`, name).Scan(&outboundJSON); err != nil {
		t.Fatal(err)
	}
	return outboundJSON
}

func assertRuntimeOutboundParses(t *testing.T, outboundJSON string) {
	t.Helper()
	if err := singboxinfra.ValidateOutboundJSON(outboundJSON); err != nil {
		t.Fatalf("runtime outbound parse failed: %v; outbound_json=%s", err, outboundJSON)
	}
}
