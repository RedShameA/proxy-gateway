package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"proxygateway/internal/testsupport/apptest"
	"testing"
	"time"
)

func TestAccessProfileDetailAndOverviewDoNotDeadlockWithSingleDBConnection(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	handler := gw.Handler()
	doJSON := func(method, target, token string, body any) *httptest.ResponseRecorder {
		t.Helper()
		var raw []byte
		if body != nil {
			var err error
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req := httptest.NewRequest(method, target, bytes.NewReader(raw))
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	mustOK := func(rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
	}

	setupRec := doJSON(http.MethodPost, "/api/admin/setup", "", map[string]string{
		"password": "correct horse battery staple",
	})
	mustOK(setupRec)
	var setupBody struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(setupRec.Body.Bytes(), &setupBody); err != nil {
		t.Fatal(err)
	}

	nodeRec := doJSON(http.MethodPost, "/api/nodes", setupBody.Token, map[string]any{
		"name": "direct",
		"type": "direct",
	})
	mustOK(nodeRec)
	var node struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(nodeRec.Body.Bytes(), &node); err != nil {
		t.Fatal(err)
	}

	profileRec := doJSON(http.MethodPost, "/api/access-profiles", setupBody.Token, map[string]any{
		"name":               "fixed",
		"profile_identifier": "fixed-test",
		"type":               "fixed_node",
		"fixed_node_id":      node.ID,
	})
	mustOK(profileRec)
	var profile struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(profileRec.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}

	credentialRec := doJSON(http.MethodPost, "/api/access-profiles/"+profile.ID+"/proxy-credentials", setupBody.Token, map[string]string{
		"remark":   "client",
		"password": "proxy123",
	})
	mustOK(credentialRec)

	for _, target := range []string{"/api/access-profiles/" + profile.ID, "/api/overview"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set("Authorization", "Bearer "+setupBody.Token)
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			done <- rec
		}()
		select {
		case rec := <-done:
			mustOK(rec)
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("GET %s deadlocked while composing response", target)
		}
	}
}

func TestAccessProfileCanBeEditedAndDeleted(t *testing.T) {
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
		"name":          "fixed",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &created)

	editResp := patchJSON(t, srv.URL+"/api/access-profiles/"+created.ID, adminToken, map[string]any{
		"name":     "jp fastest",
		"type":     "fastest",
		"test_url": "https://example.com",
		"candidate_filter": map[string]any{
			"source_mode":         "manual",
			"source_ids":          []string{},
			"name_include":        "tokyo",
			"name_exclude":        "slow",
			"egress_country_mode": "include",
			"egress_countries":    []string{"jp"},
		},
		"candidate_limit":                  7,
		"min_evaluation_interval_seconds":  120,
		"auto_evaluation_enabled":          false,
		"auto_evaluation_interval_seconds": 600,
	})
	decodeOK(t, editResp, &struct{}{})

	var edited map[string]any
	getJSON(t, srv.URL+"/api/access-profiles/"+created.ID, adminToken, &edited)
	if edited["name"] != "jp fastest" {
		t.Fatalf("name = %v", edited["name"])
	}
	if edited["type"] != "fastest" {
		t.Fatalf("type = %v", edited["type"])
	}
	filter := edited["candidate_filter"].(map[string]any)
	countries := filter["egress_countries"].([]any)
	if len(countries) != 1 || countries[0] != "JP" {
		t.Fatalf("egress_countries = %v", filter["egress_countries"])
	}
	if filter["source_mode"] != "manual" {
		t.Fatalf("source_mode = %v", filter["source_mode"])
	}
	if filter["name_include"] != "tokyo" || filter["name_exclude"] != "slow" {
		t.Fatalf("regex filters = %v / %v", filter["name_include"], filter["name_exclude"])
	}
	if edited["auto_evaluation_enabled"] != false {
		t.Fatalf("auto_evaluation_enabled = %v", edited["auto_evaluation_enabled"])
	}
	if int(edited["candidate_limit"].(float64)) != 7 {
		t.Fatalf("candidate_limit = %v", edited["candidate_limit"])
	}
	if int(edited["config_version"].(float64)) <= 1 {
		t.Fatalf("config_version = %v, want incremented", edited["config_version"])
	}

	deleteResp := deleteRequest(t, srv.URL+"/api/access-profiles/"+created.ID, adminToken)
	decodeOK(t, deleteResp, &struct{}{})
	var list struct {
		AccessProfiles []map[string]any `json:"access_profiles"`
	}
	getJSON(t, srv.URL+"/api/access-profiles", adminToken, &list)
	for _, profile := range list.AccessProfiles {
		if profile["id"] == created.ID {
			t.Fatalf("deleted profile still listed: %v", profile)
		}
	}
}

func TestAccessProfileRejectsInvalidNameFilterRegex(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	resp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name": "bad regex",
		"type": "fastest",
		"candidate_filter": map[string]any{
			"source_mode":         "all",
			"name_include":        "[",
			"egress_country_mode": "include",
			"egress_countries":    []string{},
		},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAccessProfileSummariesExposeSwitchReason(t *testing.T) {
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
		"name":          "fixed",
		"type":          "fixed_node",
		"fixed_node_id": node.ID,
	})
	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, profileResp, &created)

	var list struct {
		Items []map[string]any `json:"items"`
	}
	getJSON(t, srv.URL+"/api/access-profiles", adminToken, &list)
	assertSummaryHasSwitchReason(t, list.Items, created.ID)

	var overview struct {
		AccessProfiles []map[string]any `json:"access_profiles"`
	}
	getJSON(t, srv.URL+"/api/overview", adminToken, &overview)
	assertSummaryHasSwitchReason(t, overview.AccessProfiles, created.ID)
}

func assertSummaryHasSwitchReason(t *testing.T, profiles []map[string]any, profileID string) {
	t.Helper()
	for _, profile := range profiles {
		if profile["id"] != profileID {
			continue
		}
		if _, ok := profile["switch_reason"]; !ok {
			t.Fatalf("profile summary missing switch_reason: %#v", profile)
		}
		return
	}
	t.Fatalf("profile %s not found in summaries: %#v", profileID, profiles)
}

func TestProxyCredentialsCanBeListedAndDeleted(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("direct target"))
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

	credentialResp := postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "delete-me.client",
		"password": "proxy-password-123",
	})
	var createdCredential struct {
		ID            string `json:"id"`
		HTTPProxyURL  string `json:"http_proxy_url"`
		HTTPSProxyURL string `json:"https_proxy_url"`
		SOCKS5URL     string `json:"socks5_proxy_url"`
	}
	decodeOK(t, credentialResp, &createdCredential)
	if createdCredential.HTTPProxyURL == "" || createdCredential.HTTPSProxyURL == "" || createdCredential.SOCKS5URL == "" {
		t.Fatalf("created credential proxy urls = %#v, want http/https/socks5", createdCredential)
	}

	var credentials struct {
		ProxyCredentials []map[string]any `json:"proxy_credentials"`
	}
	getJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, &credentials)
	if len(credentials.ProxyCredentials) != 1 {
		t.Fatalf("credentials len = %d, want 1", len(credentials.ProxyCredentials))
	}
	listed := credentials.ProxyCredentials[0]
	if listed["id"] != createdCredential.ID || listed["remark"] != "delete-me.client" {
		t.Fatalf("listed credential = %v", listed)
	}
	if listed["http_proxy_url"] == "" || listed["https_proxy_url"] == "" || listed["socks5_proxy_url"] == "" {
		t.Fatalf("listed credential proxy urls = %#v, want http/https/socks5", listed)
	}
	if _, ok := listed["password_hash"]; ok {
		t.Fatal("credential list exposed password_hash")
	}

	deleteCredentialResp := deleteRequest(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials/"+createdCredential.ID, adminToken)
	decodeOK(t, deleteCredentialResp, &struct{}{})

	getJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, &credentials)
	if len(credentials.ProxyCredentials) != 0 {
		t.Fatalf("credentials after delete = %v", credentials.ProxyCredentials)
	}

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("status = %d, want 407 after credential delete", resp.StatusCode)
	}

	credentialResp = postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "profile-delete.client",
		"password": "proxy-password-456",
	})
	decodeOK(t, credentialResp, &createdCredential)
	deleteProfileResp := deleteRequest(t, srv.URL+"/api/access-profiles/"+profile.ID, adminToken)
	decodeOK(t, deleteProfileResp, &struct{}{})

	proxyURL.User = url.UserPassword(profile.ID, "proxy-password-456")
	resp, err = client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusProxyAuthRequired {
		t.Fatalf("status = %d, want 407 after profile delete", resp.StatusCode)
	}

	missingListResp := get(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken)
	var missingBody map[string]any
	decodeJSON(t, missingListResp, &missingBody)
	if missingListResp.StatusCode != http.StatusNotFound {
		t.Fatalf("credential list status = %d, body = %s", missingListResp.StatusCode, mustJSON(t, missingBody))
	}
}

func findProfileMap(t *testing.T, profiles []map[string]any, id string) map[string]any {
	t.Helper()
	for _, profile := range profiles {
		if profile["id"] == id {
			return profile
		}
	}
	t.Fatalf("profile %s not found in %v", id, profiles)
	return nil
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
