package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"proxygateway/internal/app"
)

func TestNodeObservationRecordsEgressCountry(t *testing.T) {
	t.Parallel()

	egress := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ip":"203.0.113.10","country":"US"}`))
	}))
	t.Cleanup(egress.Close)

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	nodeResp := postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "direct-observed",
		"type": "direct",
	})
	var node struct {
		ID string `json:"id"`
	}
	decodeOK(t, nodeResp, &node)

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
	if len(nodes.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(nodes.Nodes))
	}
	observation := nodes.Nodes[0]["observation"].(map[string]any)
	if observation["usable"] != true {
		t.Fatalf("usable = %v, want true", observation["usable"])
	}
	if observation["egress_ip"] != "203.0.113.10" || observation["egress_country"] != "US" {
		t.Fatalf("unexpected observation: %#v", observation)
	}
}

func TestNodeObservationAcceptsProbeURLWithoutTestURLAndParsesCloudflareTrace(t *testing.T) {
	t.Parallel()

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fl=1\nip=203.0.113.44\nloc=JP\n"))
	}))
	t.Cleanup(probe.Close)

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	decodeOK(t, postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "direct-cloudflare-trace",
		"type": "direct",
	}), &struct{}{})

	runResp := postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"probe_url": probe.URL,
	})
	var run struct {
		ObservedNodes int `json:"observed_nodes"`
	}
	decodeOK(t, runResp, &run)
	if run.ObservedNodes != 1 {
		t.Fatalf("observed_nodes = %d, want 1", run.ObservedNodes)
	}

	observations := observationsByNodeName(t, srv.URL, adminToken)
	observation := observations["direct-cloudflare-trace"]
	if observation["egress_ip"] != "203.0.113.44" || observation["egress_country"] != "JP" {
		t.Fatalf("unexpected observation from trace: %#v", observation)
	}
	if observation["stale"] != false {
		t.Fatalf("stale = %v, want false", observation["stale"])
	}
}

func TestNodeWithoutObservationHasNoLastObservedAt(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	decodeOK(t, postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "direct-never-observed",
		"type": "direct",
	}), &struct{}{})

	nodes := nodesByName(t, srv.URL, adminToken)
	node := nodes["direct-never-observed"]
	if node["state"] != "pending_observation" {
		t.Fatalf("state = %v, want pending_observation: %#v", node["state"], node)
	}
	if node["last_observed_at"] != nil {
		t.Fatalf("last_observed_at = %v, want nil for never-observed node: %#v", node["last_observed_at"], node)
	}
}

func TestNodeObservationFailureSetsLastObservedAt(t *testing.T) {
	t.Parallel()

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "probe down", http.StatusBadGateway)
	}))
	t.Cleanup(probe.Close)

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	decodeOK(t, postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "direct-failed-observation",
		"type": "direct",
	}), &struct{}{})
	decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"probe_url": probe.URL,
	}), &struct{}{})

	nodes := nodesByName(t, srv.URL, adminToken)
	node := nodes["direct-failed-observation"]
	if node["state"] != "unusable" {
		t.Fatalf("state = %v, want unusable: %#v", node["state"], node)
	}
	if got := unixFromJSON(node["last_observed_at"]); got <= 0 {
		t.Fatalf("last_observed_at = %v, want failed observation timestamp: %#v", node["last_observed_at"], node)
	}
}

func TestNodeObservationFailureKeepsLastEgressAndMarksStale(t *testing.T) {
	t.Parallel()

	failProbe := false
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failProbe {
			http.Error(w, "probe down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("ip=198.51.100.20\nloc=SG\n"))
	}))
	t.Cleanup(probe.Close)

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	decodeOK(t, postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"name": "direct-stale-observation",
		"type": "direct",
	}), &struct{}{})
	decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"probe_url": probe.URL,
	}), &struct{}{})

	failProbe = true
	decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"probe_url": probe.URL,
	}), &struct{}{})

	observations := observationsByNodeName(t, srv.URL, adminToken)
	observation := observations["direct-stale-observation"]
	if observation["usable"] != false {
		t.Fatalf("usable = %v, want false after failed observation", observation["usable"])
	}
	if observation["stale"] != true {
		t.Fatalf("stale = %v, want true after failed observation", observation["stale"])
	}
	if observation["egress_ip"] != "198.51.100.20" || observation["egress_country"] != "SG" {
		t.Fatalf("failed observation should preserve last egress data: %#v", observation)
	}
	if observation["last_error"] == "" {
		t.Fatalf("last_error should be visible after failed observation: %#v", observation)
	}

	nodes := nodesByName(t, srv.URL, adminToken)
	node := nodes["direct-stale-observation"]
	lastObservedAt := unixFromJSON(node["last_observed_at"])
	lastSuccessAt := unixFromJSON(observation["last_success_at"])
	lastFailureAt := unixFromJSON(observation["last_failure_at"])
	wantObservedAt := lastSuccessAt
	if lastFailureAt > wantObservedAt {
		wantObservedAt = lastFailureAt
	}
	if lastObservedAt != wantObservedAt || lastFailureAt <= 0 {
		t.Fatalf("last_observed_at = %d, want latest success/failure %d; observation=%#v node=%#v", lastObservedAt, wantObservedAt, observation, node)
	}
}

func nodesByName(t *testing.T, baseURL, adminToken string) map[string]map[string]any {
	t.Helper()
	var nodes struct {
		Nodes []map[string]any `json:"nodes"`
	}
	getJSON(t, baseURL+"/api/nodes", adminToken, &nodes)
	result := map[string]map[string]any{}
	for _, node := range nodes.Nodes {
		name := node["name"].(string)
		result[name] = node
	}
	return result
}

func unixFromJSON(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}
