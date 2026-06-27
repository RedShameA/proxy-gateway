package app_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"proxygateway/internal/testsupport/apptest"
	"testing"
	"time"
)

func TestChainAccessProfileCandidateStatsExcludeExitNodesFromFrontCandidates(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	createHTTPNode(t, srv.URL, adminToken, "front-node", newDelayedHTTPConnectProxy(t, 0))
	exitNodeID := createHTTPNode(t, srv.URL, adminToken, "exit-node", newDelayedHTTPConnectProxy(t, 0))

	var created struct {
		ID string `json:"id"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":          "chain-stats",
		"type":          "chain",
		"test_url":      "https://example.com/",
		"exit_node_ids": []string{exitNodeID},
	}), &created)

	var detail struct {
		CandidateStats struct {
			Total                int `json:"total"`
			Usable               int `json:"usable"`
			UnknownEgressCountry int `json:"unknown_egress_country"`
			FrontCandidates      int `json:"front_candidates"`
			ExitNodes            int `json:"exit_nodes"`
			PathCombinations     int `json:"path_combinations"`
		} `json:"candidate_stats"`
	}
	getJSON(t, srv.URL+"/api/access-profiles/"+created.ID, adminToken, &detail)
	if detail.CandidateStats.Total != 2 {
		t.Fatalf("candidate_stats.total = %d, want 2", detail.CandidateStats.Total)
	}
	if detail.CandidateStats.FrontCandidates != 1 || detail.CandidateStats.ExitNodes != 1 || detail.CandidateStats.PathCombinations != 1 {
		t.Fatalf("candidate_stats = %#v, want front=1 exit=1 path=1", detail.CandidateStats)
	}
	if detail.CandidateStats.UnknownEgressCountry != 2 {
		t.Fatalf("candidate_stats.unknown_egress_country = %d, want 2", detail.CandidateStats.UnknownEgressCountry)
	}
}

func TestCountryFilteredAccessProfileEnqueuesObservationForUnknownCandidates(t *testing.T) {
	t.Parallel()

	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeID := createHTTPNode(t, srv.URL, adminToken, "manual-unknown-country", newDelayedHTTPConnectProxy(t, 0))

	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name": "country-filtered",
		"type": "fastest",
		"candidate_filter": map[string]any{
			"source_mode":         "manual",
			"egress_country_mode": "include",
			"egress_countries":    []string{"US"},
		},
	}), &struct{}{})

	var runs struct {
		Items []struct {
			RunType       string         `json:"run_type"`
			TriggerSource string         `json:"trigger_source"`
			Detail        map[string]any `json:"detail"`
		} `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=node_observation", adminToken, &runs)
	for _, run := range runs.Items {
		if run.RunType == "node_observation" && run.TriggerSource == "country_profile_unknown_country" {
			if run.Detail["target_scope"] != "all_nodes" {
				t.Fatalf("target_scope = %#v, want all_nodes", run.Detail["target_scope"])
			}
			nodeIDs, ok := run.Detail["node_ids"].([]any)
			if !ok || len(nodeIDs) != 1 || nodeIDs[0] != nodeID {
				t.Fatalf("node_ids = %#v, want [%s]", run.Detail["node_ids"], nodeID)
			}
			return
		}
	}
	t.Fatalf("country_profile_unknown_country run not found: %#v", runs.Items)
}

func TestChainCandidateFilterAppliesOnlyToFrontNodes(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("chain front only filter"))
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	exitProxy := newDelayedHTTPConnectProxy(t, 0)
	frontHTTP := newDelayedHTTPConnectProxy(t, 80*time.Millisecond)
	frontSOCKS := newDelayedSOCKS5Proxy(t, 0)
	gw := apptest.NewGateway(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	exitNodeID := createHTTPNode(t, srv.URL, adminToken, "protocol-exit-http", exitProxy)
	createHTTPNode(t, srv.URL, adminToken, "protocol-front-http", frontHTTP)
	frontSOCKSNodeID := createSOCKS5Node(t, srv.URL, adminToken, "protocol-front-socks", frontSOCKS)

	var profile struct {
		ID string `json:"id"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":                  "chain-front-only-filter",
		"type":                  "chain",
		"exit_node_ids":         []string{exitNodeID},
		"chain_evaluation_mode": "end_to_end",
		"test_url":              target.URL,
		"candidate_filter": map[string]any{
			"protocols": []string{"socks5"},
		},
	}), &profile)

	decodeOK(t, postJSON(t, srv.URL+"/api/evaluations/run", adminToken, map[string]any{}), &struct{}{})
	assertProfileCurrentNode(t, srv.URL, adminToken, profile.ID, frontSOCKSNodeID, "ready")

	const password = "proxy-password-123"
	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "front-only.client",
		"password": password,
	}), &struct{}{})
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword(profile.ID, password)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/front-only-filter")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if exitProxy.connects == 0 {
		t.Fatal("exit node should still be used even when it does not match candidate_filter.protocols")
	}
}
