package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"proxygateway/internal/app"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
	"proxygateway/internal/testsupport/apptest"
	"testing"
	"time"
)

func TestListNodesDoesNotDeadlockWithSingleDBConnection(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	handler := gw.Handler()
	doJSON := func(method, target, token string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, target, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	setupRec := doJSON(http.MethodPost, "/api/admin/setup", "", map[string]string{
		"password": "correct horse battery staple",
	})
	if setupRec.Code != http.StatusOK {
		t.Fatalf("setup status = %d: %s", setupRec.Code, setupRec.Body.String())
	}
	var setupBody struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(setupRec.Body.Bytes(), &setupBody); err != nil {
		t.Fatal(err)
	}

	nodeRec := doJSON(http.MethodPost, "/api/nodes", setupBody.Token, map[string]any{
		"name":        "single-conn-node",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 19090,
	})
	if nodeRec.Code != http.StatusOK {
		t.Fatalf("create node status = %d: %s", nodeRec.Code, nodeRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/nodes?page=1&page_size=10", nil)
	req.Header.Set("Authorization", "Bearer "+setupBody.Token)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		done <- rec
	}()

	select {
	case rec := <-done:
		if rec.Code != http.StatusOK {
			t.Fatalf("list nodes status = %d: %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Items []map[string]any `json:"items"`
			Total int              `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Total != 1 || len(body.Items) != 1 {
			t.Fatalf("nodes response total/items = %d/%d: %s", body.Total, len(body.Items), rec.Body.String())
		}
		if _, ok := body.Items[0]["observation"]; !ok {
			t.Fatalf("node response missing observation: %s", rec.Body.String())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("GET /api/nodes deadlocked while listing node details")
	}
}

func TestListNodesFiltersBeforePagination(t *testing.T) {
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
	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	createNode := func(body map[string]any) string {
		t.Helper()
		resp := postJSON(t, srv.URL+"/api/nodes", adminToken, body)
		var created struct {
			ID string `json:"id"`
		}
		decodeOK(t, resp, &created)
		return created.ID
	}
	createNode(map[string]any{"name": "filter-alpha", "type": "http", "server": "127.0.0.1", "server_port": 19081})
	disabledID := createNode(map[string]any{"name": "filter-beta", "type": "http", "server": "127.0.0.1", "server_port": 19082})
	unusableID := createNode(map[string]any{"name": "filter-gamma", "type": "socks5", "server": "127.0.0.1", "server_port": 19083})
	decodeOK(t, patchJSON(t, srv.URL+"/api/nodes/"+disabledID, adminToken, map[string]any{"enabled": false}), &struct{}{})
	if _, err := db.Exec(
		`INSERT INTO node_observations (node_id, usable, last_error, last_failure_at) VALUES (?, 0, ?, ?)`,
		unusableID,
		"dial failed",
		time.Now().UnixMilli(),
	); err != nil {
		t.Fatal(err)
	}

	var disabled struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	getJSON(t, srv.URL+"/api/nodes?state=disabled&page=1&page_size=1", adminToken, &disabled)
	if disabled.Total != 1 || len(disabled.Items) != 1 || disabled.Items[0]["id"] != disabledID {
		t.Fatalf("disabled filter response = %#v", disabled)
	}

	var unusable struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	getJSON(t, srv.URL+"/api/nodes?state=unusable&page=1&page_size=10", adminToken, &unusable)
	if unusable.Total != 1 || len(unusable.Items) != 1 || unusable.Items[0]["id"] != unusableID {
		t.Fatalf("unusable filter response = %#v", unusable)
	}

	var secondPage struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	getJSON(t, srv.URL+"/api/nodes?name=filter&page=2&page_size=1", adminToken, &secondPage)
	if secondPage.Total != 3 || len(secondPage.Items) != 1 {
		t.Fatalf("name filter pagination response = %#v", secondPage)
	}

	var socksNodes struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	getJSON(t, srv.URL+"/api/nodes?protocol=socks5&page=1&page_size=10", adminToken, &socksNodes)
	if socksNodes.Total != 1 || len(socksNodes.Items) != 1 || socksNodes.Items[0]["protocol"] != "socks5" {
		t.Fatalf("protocol filter response = %#v", socksNodes)
	}
}

func TestNodeDeduplicationAcrossSources(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	manualResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-shared",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 19090,
	})
	decodeOK(t, manualResp, &struct{}{})

	content := `{"outbounds":[{"type":"http","tag":"sub-shared","server":"127.0.0.1","server_port":19090}]}`
	for _, name := range []string{"sub-a", "sub-b"} {
		resp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
			"name":        name,
			"source_type": "local",
			"content":     content,
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
		if node["server"] == "127.0.0.1" && node["server_port"].(float64) == 19090 {
			matches++
			sources = node["sources"].([]any)
		}
	}
	if matches != 1 {
		t.Fatalf("deduplicated node matches = %d, want 1; nodes=%#v", matches, nodes.Nodes)
	}
	if len(sources) != 3 {
		t.Fatalf("sources = %d, want 3: %#v", len(sources), sources)
	}
}

func TestManualNodeImportSupportsURIOutboundJSONAndDedup(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	uriResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"import_text": "http://user:pass@127.0.0.1:19080#manual-http-uri",
	})
	var uriImport struct {
		ID         string   `json:"id"`
		IDs        []string `json:"ids"`
		Imported   int      `json:"imported_nodes"`
		Skipped    int      `json:"skipped_entries"`
		ParseError string   `json:"parse_error"`
	}
	decodeOK(t, uriResp, &uriImport)
	if uriImport.ID == "" || len(uriImport.IDs) != 1 || uriImport.Imported != 1 || uriImport.Skipped != 0 || uriImport.ParseError != "" {
		t.Fatalf("unexpected URI import response: %#v", uriImport)
	}

	jsonText := `{"type":"socks","tag":"manual-socks-json","server":"127.0.0.1","server_port":19081,"username":"u","password":"p"}`
	for i := 0; i < 2; i++ {
		resp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
			"import_text": jsonText,
		})
		decodeOK(t, resp, &struct{}{})
	}

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	manualHTTP := 0
	manualSOCKS := 0
	for _, node := range nodes.Nodes {
		switch node["name"] {
		case "manual-http-uri":
			manualHTTP++
			if node["type"] != "http" || node["server"] != "127.0.0.1" || node["server_port"].(float64) != 19080 {
				t.Fatalf("unexpected URI node: %#v", node)
			}
		case "manual-socks-json":
			manualSOCKS++
			if node["type"] != "socks5" || node["server"] != "127.0.0.1" || node["server_port"].(float64) != 19081 {
				t.Fatalf("unexpected outbound JSON node: %#v", node)
			}
		}
	}
	if manualHTTP != 1 || manualSOCKS != 1 {
		t.Fatalf("manual import node counts http/socks = %d/%d, nodes=%#v", manualHTTP, manualSOCKS, nodes.Nodes)
	}

	var uriDetail map[string]any
	getJSON(t, srv.URL+"/api/nodes/"+uriImport.ID, adminToken, &uriDetail)
	if uriDetail["raw_json"] != "http://user:pass@127.0.0.1:19080#manual-http-uri" {
		t.Fatalf("detail raw_json = %#v", uriDetail["raw_json"])
	}
	if _, ok := uriDetail["outbound_json"]; ok {
		t.Fatalf("detail should not expose outbound_json for import editing: %#v", uriDetail)
	}

	badResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"import_text": "not a proxy node",
	})
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid import status = %d, want 400", badResp.StatusCode)
	}
}

func TestDeleteManualNodeRemovesManualSourceAndKeepsSharedSubscriptionNode(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	manualResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-delete-shared",
		"type":        "socks5",
		"server":      "127.0.0.1",
		"server_port": 19082,
		"username":    "user",
		"password":    "pass",
	})
	var manualNode struct {
		ID string `json:"id"`
	}
	decodeOK(t, manualResp, &manualNode)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "shared-node-sub",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"socks","tag":"sub-delete-shared","server":"127.0.0.1","server_port":19082,"username":"user","password":"pass"}]}`,
	})
	decodeOK(t, subResp, &struct{}{})

	deleteResp := deleteRequest(t, srv.URL+"/api/nodes/"+manualNode.ID, adminToken)
	decodeOK(t, deleteResp, &struct{}{})

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	if len(nodes.Nodes) != 1 {
		t.Fatalf("shared node should remain after manual source deletion: %#v", nodes.Nodes)
	}
	sources := nodes.Nodes[0]["sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("sources after delete = %d, want subscription source only: %#v", len(sources), sources)
	}
	source := sources[0].(map[string]any)
	if source["source_type"] != "subscription" {
		t.Fatalf("remaining source = %#v, want subscription", source)
	}

	deleteAgain := deleteRequest(t, srv.URL+"/api/nodes/"+manualNode.ID, adminToken)
	if deleteAgain.StatusCode != http.StatusBadRequest {
		t.Fatalf("second manual delete status = %d, want 400", deleteAgain.StatusCode)
	}
}

func TestUpdateManualNodeEditsPureManualNodeInPlace(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-edit-original",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 19083,
		"username":    "old-user",
		"password":    "old-pass",
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	updateResp := patchJSON(t, srv.URL+"/api/nodes/"+created.ID, adminToken, map[string]any{
		"name":        "manual-edit-updated",
		"type":        "socks5",
		"server":      "127.0.0.2",
		"server_port": 19084,
		"username":    "new-user",
		"password":    "new-pass",
	})
	var updated struct {
		Updated bool   `json:"updated"`
		ID      string `json:"id"`
		Split   bool   `json:"split"`
	}
	decodeOK(t, updateResp, &updated)
	if !updated.Updated || updated.ID != created.ID || updated.Split {
		t.Fatalf("unexpected update response: %#v", updated)
	}

	node := getNodeMapByID(t, srv.URL, adminToken, created.ID)
	if node["name"] != "manual-edit-updated" ||
		node["type"] != "socks5" ||
		node["protocol"] != "socks5" ||
		node["server"] != "127.0.0.2" ||
		node["server_port"].(float64) != 19084 ||
		node["username"] != "new-user" ||
		node["password"] != "new-pass" {
		t.Fatalf("node was not updated in place: %#v", node)
	}
	sources := node["sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("sources = %d, want 1: %#v", len(sources), sources)
	}
	source := sources[0].(map[string]any)
	if source["source_type"] != "manual" || source["display_name"] != "manual-edit-updated" {
		t.Fatalf("manual source not updated: %#v", source)
	}

	var detail map[string]any
	getJSON(t, srv.URL+"/api/nodes/"+created.ID, adminToken, &detail)
	if detail["username"] != "new-user" || detail["password"] != "new-pass" {
		t.Fatalf("detail credentials were not exposed for editing: %#v", detail)
	}

	var runs struct {
		Items []map[string]any `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=node_observation&target_id="+created.ID, adminToken, &runs)
	if len(runs.Items) == 0 {
		t.Fatal("expected node observation run after manual node edit")
	}
	found := false
	for _, run := range runs.Items {
		if run["trigger_source"] == "manual_node_import" && run["target_label"] == "manual-edit-updated" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unexpected observation runs after edit: %#v", runs.Items)
	}
}

func TestUpdateManualNodeWithImportTextSupportsHTTPAndSOCKS5(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-uri-edit-original",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 19092,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	updateSOCKSResp := patchJSON(t, srv.URL+"/api/nodes/"+created.ID, adminToken, map[string]any{
		"import_text": "socks5://uri-user:uri-pass@127.0.0.2:19093#manual-uri-socks",
	})
	var updatedSOCKS struct {
		Updated bool   `json:"updated"`
		ID      string `json:"id"`
		Split   bool   `json:"split"`
	}
	decodeOK(t, updateSOCKSResp, &updatedSOCKS)
	if !updatedSOCKS.Updated || updatedSOCKS.ID != created.ID || updatedSOCKS.Split {
		t.Fatalf("unexpected socks import_text update response: %#v", updatedSOCKS)
	}
	socksNode := getNodeMapByID(t, srv.URL, adminToken, created.ID)
	if socksNode["name"] != "manual-uri-socks" ||
		socksNode["type"] != "socks5" ||
		socksNode["server"] != "127.0.0.2" ||
		socksNode["server_port"].(float64) != 19093 ||
		socksNode["username"] != "uri-user" ||
		socksNode["password"] != "uri-pass" {
		t.Fatalf("unexpected socks import_text node: %#v", socksNode)
	}

	updateHTTPResp := patchJSON(t, srv.URL+"/api/nodes/"+created.ID, adminToken, map[string]any{
		"import_text": "http://http-user:http-pass@127.0.0.3:19094#manual-uri-http",
	})
	var updatedHTTP struct {
		Updated bool   `json:"updated"`
		ID      string `json:"id"`
		Split   bool   `json:"split"`
	}
	decodeOK(t, updateHTTPResp, &updatedHTTP)
	if !updatedHTTP.Updated || updatedHTTP.ID != created.ID || updatedHTTP.Split {
		t.Fatalf("unexpected http import_text update response: %#v", updatedHTTP)
	}
	httpNode := getNodeMapByID(t, srv.URL, adminToken, created.ID)
	if httpNode["name"] != "manual-uri-http" ||
		httpNode["type"] != "http" ||
		httpNode["server"] != "127.0.0.3" ||
		httpNode["server_port"].(float64) != 19094 ||
		httpNode["username"] != "http-user" ||
		httpNode["password"] != "http-pass" {
		t.Fatalf("unexpected http import_text node: %#v", httpNode)
	}
}

func TestUpdateManualNodeWithImportTextSupportsNonFormNodeTypes(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"import_text": `{"type":"shadowsocks","tag":"manual-ss-original","server":"127.0.0.1","server_port":19095,"method":"aes-128-gcm","password":"old-secret"}`,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	updateResp := patchJSON(t, srv.URL+"/api/nodes/"+created.ID, adminToken, map[string]any{
		"import_text": `{"type":"shadowsocks","tag":"manual-ss-updated","server":"127.0.0.2","server_port":19096,"method":"aes-128-gcm","password":"new-secret"}`,
	})
	var updated struct {
		Updated bool   `json:"updated"`
		ID      string `json:"id"`
		Split   bool   `json:"split"`
	}
	decodeOK(t, updateResp, &updated)
	if !updated.Updated || updated.ID != created.ID || updated.Split {
		t.Fatalf("unexpected shadowsocks import_text update response: %#v", updated)
	}
	node := getNodeMapByID(t, srv.URL, adminToken, created.ID)
	if node["name"] != "manual-ss-updated" ||
		node["type"] != "shadowsocks" ||
		node["server"] != "127.0.0.2" ||
		node["server_port"].(float64) != 19096 ||
		node["password"] != "new-secret" {
		t.Fatalf("unexpected shadowsocks import_text node: %#v", node)
	}
}

func TestUpdateManualNodeWithImportTextRejectsMultipleNodes(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-multi-reject",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 19097,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	updateResp := patchJSON(t, srv.URL+"/api/nodes/"+created.ID, adminToken, map[string]any{
		"import_text": "http://127.0.0.2:19098#one\nhttp://127.0.0.3:19099#two",
	})
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("multi-node import_text edit status = %d, want 400", updateResp.StatusCode)
	}
}

func TestUpdateManualSharedNodeSplitsFromSubscriptionNode(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-shared-edit",
		"type":        "socks5",
		"server":      "127.0.0.1",
		"server_port": 19085,
		"username":    "shared-user",
		"password":    "shared-pass",
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "shared-edit-sub",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"socks","tag":"sub-shared-edit","server":"127.0.0.1","server_port":19085,"username":"shared-user","password":"shared-pass"}]}`,
	})
	decodeOK(t, subResp, &struct{}{})

	sharedNode := getNodeMapByID(t, srv.URL, adminToken, created.ID)
	if len(sharedNode["sources"].([]any)) != 2 {
		t.Fatalf("node should have manual and subscription sources before edit: %#v", sharedNode)
	}

	updateResp := patchJSON(t, srv.URL+"/api/nodes/"+created.ID, adminToken, map[string]any{
		"name":        "manual-split-updated",
		"type":        "http",
		"server":      "127.0.0.2",
		"server_port": 19086,
		"username":    "split-user",
		"password":    "split-pass",
	})
	var updated struct {
		Updated bool   `json:"updated"`
		ID      string `json:"id"`
		Split   bool   `json:"split"`
	}
	decodeOK(t, updateResp, &updated)
	if !updated.Updated || updated.ID == "" || updated.ID == created.ID || !updated.Split {
		t.Fatalf("unexpected split update response: %#v", updated)
	}

	oldNode := getNodeMapByID(t, srv.URL, adminToken, created.ID)
	oldSources := oldNode["sources"].([]any)
	if len(oldSources) != 1 || oldSources[0].(map[string]any)["source_type"] != "subscription" {
		t.Fatalf("old node should keep only subscription source: %#v", oldNode)
	}
	newNode := getNodeMapByID(t, srv.URL, adminToken, updated.ID)
	newSources := newNode["sources"].([]any)
	if len(newSources) != 1 || newSources[0].(map[string]any)["source_type"] != "manual" {
		t.Fatalf("new node should have only manual source: %#v", newNode)
	}
	if newNode["name"] != "manual-split-updated" ||
		newNode["type"] != "http" ||
		newNode["server"] != "127.0.0.2" ||
		newNode["server_port"].(float64) != 19086 ||
		newNode["username"] != "split-user" ||
		newNode["password"] != "split-pass" {
		t.Fatalf("new split node has unexpected fields: %#v", newNode)
	}

	var runs struct {
		Items []map[string]any `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=node_observation&target_id="+updated.ID, adminToken, &runs)
	if len(runs.Items) == 0 || runs.Items[0]["trigger_source"] != "manual_node_import" {
		t.Fatalf("expected observation run for split manual node: %#v", runs.Items)
	}
}

func TestUpdateManualSharedNodeWithImportTextSplitsFromSubscriptionNode(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-shared-uri-edit",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 19100,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "shared-uri-edit-sub",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"http","tag":"sub-shared-uri-edit","server":"127.0.0.1","server_port":19100}]}`,
	})
	decodeOK(t, subResp, &struct{}{})

	updateResp := patchJSON(t, srv.URL+"/api/nodes/"+created.ID, adminToken, map[string]any{
		"import_text": "socks5://split-uri-user:split-uri-pass@127.0.0.2:19101#manual-uri-split",
	})
	var updated struct {
		Updated bool   `json:"updated"`
		ID      string `json:"id"`
		Split   bool   `json:"split"`
	}
	decodeOK(t, updateResp, &updated)
	if !updated.Updated || updated.ID == "" || updated.ID == created.ID || !updated.Split {
		t.Fatalf("unexpected import_text split response: %#v", updated)
	}

	oldNode := getNodeMapByID(t, srv.URL, adminToken, created.ID)
	oldSources := oldNode["sources"].([]any)
	if len(oldSources) != 1 || oldSources[0].(map[string]any)["source_type"] != "subscription" {
		t.Fatalf("old node should keep only subscription source: %#v", oldNode)
	}
	newNode := getNodeMapByID(t, srv.URL, adminToken, updated.ID)
	newSources := newNode["sources"].([]any)
	if len(newSources) != 1 || newSources[0].(map[string]any)["source_type"] != "manual" {
		t.Fatalf("new node should have only manual source: %#v", newNode)
	}
	if newNode["name"] != "manual-uri-split" ||
		newNode["type"] != "socks5" ||
		newNode["server"] != "127.0.0.2" ||
		newNode["server_port"].(float64) != 19101 ||
		newNode["username"] != "split-uri-user" ||
		newNode["password"] != "split-uri-pass" {
		t.Fatalf("new split import_text node has unexpected fields: %#v", newNode)
	}
}

func TestUpdateSubscriptionOnlyNodeIsForbidden(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "subscription-only-edit",
		"source_type": "local",
		"content":     `{"outbounds":[{"type":"http","tag":"sub-only","server":"127.0.0.1","server_port":19087}]}`,
	})
	decodeOK(t, subResp, &struct{}{})

	node := getOnlyNodeMap(t, srv.URL, adminToken)
	updateResp := patchJSON(t, srv.URL+"/api/nodes/"+node["id"].(string), adminToken, map[string]any{
		"name":        "should-not-edit",
		"type":        "http",
		"server":      "127.0.0.2",
		"server_port": 19088,
	})
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("subscription-only edit status = %d, want 400", updateResp.StatusCode)
	}

	updateImportResp := patchJSON(t, srv.URL+"/api/nodes/"+node["id"].(string), adminToken, map[string]any{
		"import_text": "http://127.0.0.3:19102#should-not-edit-import",
	})
	defer updateImportResp.Body.Close()
	if updateImportResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("subscription-only import_text edit status = %d, want 400", updateImportResp.StatusCode)
	}
}

func TestUpdatePureManualNodeRejectsDuplicateFingerprint(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	firstResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-duplicate-first",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 19089,
	})
	var first struct {
		ID string `json:"id"`
	}
	decodeOK(t, firstResp, &first)

	secondResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-duplicate-second",
		"type":        "socks5",
		"server":      "127.0.0.2",
		"server_port": 19090,
		"username":    "dupe-user",
		"password":    "dupe-pass",
	})
	decodeOK(t, secondResp, &struct{}{})

	updateResp := patchJSON(t, srv.URL+"/api/nodes/"+first.ID, adminToken, map[string]any{
		"name":        "manual-duplicate-attempt",
		"type":        "socks5",
		"server":      "127.0.0.2",
		"server_port": 19090,
		"username":    "dupe-user",
		"password":    "dupe-pass",
	})
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate edit status = %d, want 409", updateResp.StatusCode)
	}

	importTextResp := patchJSON(t, srv.URL+"/api/nodes/"+first.ID, adminToken, map[string]any{
		"import_text": "socks5://dupe-user:dupe-pass@127.0.0.2:19090#manual-duplicate-attempt",
	})
	defer importTextResp.Body.Close()
	if importTextResp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate import_text edit status = %d, want 409", importTextResp.StatusCode)
	}
}

func TestUpdateNodeEnabledStillWorks(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name":        "manual-enable-toggle",
		"type":        "http",
		"server":      "127.0.0.1",
		"server_port": 19091,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, createResp, &created)

	updateResp := patchJSON(t, srv.URL+"/api/nodes/"+created.ID, adminToken, map[string]any{
		"enabled": false,
	})
	var updated struct {
		Updated bool   `json:"updated"`
		ID      string `json:"id"`
		Split   bool   `json:"split"`
	}
	decodeOK(t, updateResp, &updated)
	if !updated.Updated || updated.ID != created.ID || updated.Split {
		t.Fatalf("unexpected enable update response: %#v", updated)
	}
	node := getNodeMapByID(t, srv.URL, adminToken, created.ID)
	if node["enabled"] != false || node["state"] != "disabled" {
		t.Fatalf("node was not disabled: %#v", node)
	}
}

func TestNodeListCanFilterByEgressCountryUsabilityAndName(t *testing.T) {
	t.Parallel()

	usProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "US")
	jpProxy := newCountryHTTPConnectProxy(t, "egress.local:80", "JP")
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createHTTPNode(t, srv.URL, adminToken, "alpha-us", usProxy)
	createHTTPNode(t, srv.URL, adminToken, "beta-jp", jpProxy)
	observeResp := postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"test_url": "http://egress.local/",
	})
	decodeOK(t, observeResp, &struct{}{})

	var filtered struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes?egress_country=US&usable=true&name=alpha", adminToken, &filtered)
	if len(filtered.Nodes) != 1 {
		t.Fatalf("filtered node count = %d, want 1: %#v", len(filtered.Nodes), filtered.Nodes)
	}
	if filtered.Nodes[0]["name"] != "alpha-us" {
		t.Fatalf("filtered node name = %v, want alpha-us", filtered.Nodes[0]["name"])
	}

	var unavailable struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes?usable=false", adminToken, &unavailable)
	if len(unavailable.Nodes) != 0 {
		t.Fatalf("usable=false should not include observed usable nodes: %#v", unavailable.Nodes)
	}
}

func TestLegacyNodeMigrationNormalizesOutboundJSONDedupesAndKeepsFixedProfileUsable(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("through migrated node"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	upstreamProxy := newHTTPConnectProxy(t)
	dataDir := t.TempDir()
	legacyNodeID := "node_legacy_http"
	seedLegacyNodeDB(t, dataDir, legacyNodeID, upstreamProxy)

	gw, err := app.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	gw, err = app.New(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	subResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "equivalent-subscription",
		"source_type": "local",
		"content": fmt.Sprintf(`{"outbounds":[{"tag":"subscription-alias","server_port":%d,"server":"%s","type":"http"}]}`,
			upstreamProxy.port,
			upstreamProxy.host,
		),
	})
	decodeOK(t, subResp, &struct{}{})

	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, srv.URL+"/api/nodes", adminToken, &nodes)
	matches := 0
	var sources []any
	for _, node := range nodes.Nodes {
		if node["server"] == upstreamProxy.host && node["server_port"].(float64) == float64(upstreamProxy.port) {
			matches++
			if node["id"] != legacyNodeID {
				t.Fatalf("deduped node id = %v, want legacy id %s", node["id"], legacyNodeID)
			}
			sources = node["sources"].([]any)
		}
	}
	if matches != 1 {
		t.Fatalf("deduplicated migrated node matches = %d, want 1; nodes=%#v", matches, nodes.Nodes)
	}
	if len(sources) != 2 {
		t.Fatalf("sources = %d, want legacy manual source and imported source: %#v", len(sources), sources)
	}
	outboundJSON, fingerprint := readNodeStorage(t, dataDir, legacyNodeID)
	wantOutboundJSON := fmt.Sprintf(`{"server":"%s","server_port":%d,"type":"http"}`, upstreamProxy.host, upstreamProxy.port)
	if outboundJSON != wantOutboundJSON {
		t.Fatalf("outbound_json = %s, want %s", outboundJSON, wantOutboundJSON)
	}
	if fingerprint == "legacy-fingerprint-not-normalized" {
		t.Fatal("fingerprint was not migrated to normalized outbound_json fingerprint")
	}

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "legacy-fixed",
		"type":          "fixed_node",
		"fixed_node_id": legacyNodeID,
	})
	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &profile)
	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "legacy.client",
		"password": "proxy-password-123",
	})
	decodeOK(t, credentialResp, &struct{}{})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/legacy")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "through migrated node" {
		t.Fatalf("body = %q", body)
	}
	if upstreamProxy.connects == 0 {
		t.Fatal("fixed profile did not use migrated legacy node")
	}
}

func getOnlyNodeMap(t *testing.T, baseURL, adminToken string) map[string]any {
	t.Helper()
	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, baseURL+"/api/nodes", adminToken, &nodes)
	if len(nodes.Nodes) != 1 {
		t.Fatalf("node count = %d, want 1: %#v", len(nodes.Nodes), nodes.Nodes)
	}
	return nodes.Nodes[0]
}

func getNodeMapByID(t *testing.T, baseURL, adminToken, nodeID string) map[string]any {
	t.Helper()
	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, baseURL+"/api/nodes", adminToken, &nodes)
	for _, node := range nodes.Nodes {
		if node["id"] == nodeID {
			return node
		}
	}
	t.Fatalf("node %s not found: %#v", nodeID, nodes.Nodes)
	return nil
}

func readNodeStorage(t *testing.T, dataDir, nodeID string) (string, string) {
	t.Helper()
	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var outboundJSON, fingerprint string
	if err := db.QueryRow(`SELECT outbound_json, fingerprint FROM nodes WHERE id = ?`, nodeID).Scan(&outboundJSON, &fingerprint); err != nil {
		t.Fatal(err)
	}
	return outboundJSON, fingerprint
}

func seedLegacyNodeDB(t *testing.T, dataDir, nodeID string, upstreamProxy *testHTTPConnectProxy) {
	t.Helper()
	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE nodes (
			id TEXT PRIMARY KEY,
			fingerprint TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			server TEXT NOT NULL DEFAULT '',
			server_port INTEGER NOT NULL DEFAULT 0,
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			raw_json TEXT NOT NULL DEFAULT '',
			source_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE node_sources (
			node_id TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_name TEXT NOT NULL,
			source_type TEXT NOT NULL,
			display_name TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (node_id, source_id)
		)`,
		`INSERT INTO nodes (id, fingerprint, name, type, server, server_port, username, password, raw_json, source_id, created_at)
		 VALUES (?, 'legacy-fingerprint-not-normalized', 'legacy-http', 'http', ?, ?, '', '', '', 'manual', ?)`,
		`INSERT INTO node_sources (node_id, source_id, source_name, source_type, display_name, created_at)
		 VALUES (?, 'manual', 'Manual', 'manual', 'legacy-http', ?)`,
	}
	for _, stmt := range stmts[:2] {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UnixMilli()
	if _, err := db.Exec(stmts[2], nodeID, upstreamProxy.host, upstreamProxy.port, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(stmts[3], nodeID, now); err != nil {
		t.Fatal(err)
	}
}
