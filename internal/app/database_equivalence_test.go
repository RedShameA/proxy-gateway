package app_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"proxygateway/internal/app"
	maintenanceapp "proxygateway/internal/application/maintenance"
	"proxygateway/internal/testsupport/apptest"
)

func TestSQLiteAndPostgresProduceEquivalentCoreWorkflowResults(t *testing.T) {
	snapshots := map[string]databaseEquivalenceSnapshot{}
	cases := []struct {
		name       string
		newGateway func(*testing.T) *app.Gateway
	}{
		{
			name: "sqlite",
			newGateway: func(t *testing.T) *app.Gateway {
				return apptest.NewGateway(t, app.WithoutMaintenanceRunner())
			},
		},
		{
			name: "postgres",
			newGateway: func(t *testing.T) *app.Gateway {
				return newPostgresGatewayForAppTest(t)
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			snapshots[tc.name] = runDatabaseEquivalenceWorkflow(t, tc.newGateway(t))
		})
	}

	postgres, ok := snapshots["postgres"]
	if !ok {
		return
	}
	if !reflect.DeepEqual(snapshots["sqlite"], postgres) {
		t.Fatalf("SQLite/PG workflow snapshots differ\nsqlite:   %#v\npostgres: %#v", snapshots["sqlite"], postgres)
	}
}

type databaseEquivalenceSnapshot struct {
	SetupRequiredBefore bool
	SetupRequiredAfter  bool
	ImportedNodes       int
	ObservedNodes       int
	ObservationResult   string
	ObservationReason   string
	EvaluatedProfiles   int
	ProfileState        string
	QueuedEvalRunState  string
	QueuedEvalRunSource string
	RequestLogResult    string
	RequestLogState     string
	RequestLogSuccess   bool
}

func runDatabaseEquivalenceWorkflow(t *testing.T, gw *app.Gateway) databaseEquivalenceSnapshot {
	t.Helper()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/probe":
			_, _ = w.Write([]byte("ip=203.0.113.70\nloc=US\n"))
		case "/generate_204":
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte("equivalent proxy"))
		}
	}))
	t.Cleanup(target.Close)
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	upstreamProxy := newHTTPConnectProxy(t)

	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)

	var beforeSetup struct {
		RequiresSetup bool `json:"requires_setup"`
	}
	getJSON(t, srv.URL+"/api/system/setup-status", "", &beforeSetup)
	adminToken := setupAdmin(t, srv.URL)
	var afterSetup struct {
		RequiresSetup bool `json:"requires_setup"`
	}
	getJSON(t, srv.URL+"/api/system/setup-status", "", &afterSetup)

	subscriptionResp := postJSON(t, srv.URL+"/api/subscriptions", adminToken, map[string]any{
		"name":        "equivalence-subscription",
		"source_type": "local",
		"content": fmt.Sprintf(
			`{"outbounds":[{"type":"http","tag":"equivalence-http","server":%q,"server_port":%d}]}`,
			upstreamProxy.host,
			upstreamProxy.port,
		),
	})
	var subscription struct {
		ImportedNodes int `json:"imported_nodes"`
	}
	decodeOK(t, subscriptionResp, &subscription)
	if subscription.ImportedNodes != 1 {
		t.Fatalf("imported_nodes = %d, want 1", subscription.ImportedNodes)
	}

	var observation struct {
		ObservedNodes int `json:"observed_nodes"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/nodes/observations/run", adminToken, map[string]string{
		"probe_url": target.URL + "/probe",
	}), &observation)
	if observation.ObservedNodes != 1 {
		t.Fatalf("observed_nodes = %d, want 1", observation.ObservedNodes)
	}

	profileResp := postJSON(t, srv.URL+"/api/access-profiles", adminToken, map[string]any{
		"name":               "equivalence fastest",
		"profile_identifier": "equivalence-fastest",
		"type":               "fastest",
		"test_url":           target.URL + "/generate_204",
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

	decodeOK(t, postJSON(t, srv.URL+"/api/access-profiles/"+profile.ID+"/proxy-credentials", adminToken, map[string]string{
		"remark":   "equivalence client",
		"password": "proxy-password-123",
	}), &struct{}{})

	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword("equivalence-fastest", "proxy-password-123")
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	resp, err := client.Get("http://" + targetURL.Host + "/proxy")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "equivalent proxy" {
		t.Fatalf("proxied body = %q, want equivalent proxy", body)
	}

	requestLogs := waitForRequestLogs(t, srv.URL+"/api/request-logs?access_profile_id="+url.QueryEscape(profile.ID)+"&target="+url.QueryEscape(targetURL.Host)+"&success=true", adminToken, 1)
	requestLog := requestLogs[0]
	success, _ := requestLog["success"].(bool)

	var profileDetail struct {
		State string `json:"state"`
	}
	getJSON(t, srv.URL+"/api/access-profiles/"+profile.ID, adminToken, &profileDetail)
	observationRun := latestMaintenanceRunForAppTest(t, srv.URL, adminToken, maintenanceapp.RunTypeNodeObservation)
	var profileRuns struct {
		Items []maintenanceRunForAppTest `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type="+maintenanceapp.RunTypeProfileEvaluation+"&target_id="+profile.ID, adminToken, &profileRuns)
	if len(profileRuns.Items) == 0 {
		t.Fatalf("profile evaluation maintenance history is empty")
	}
	queuedEvaluationRun := profileRuns.Items[0]

	return databaseEquivalenceSnapshot{
		SetupRequiredBefore: beforeSetup.RequiresSetup,
		SetupRequiredAfter:  afterSetup.RequiresSetup,
		ImportedNodes:       subscription.ImportedNodes,
		ObservedNodes:       observation.ObservedNodes,
		ObservationResult:   observationRun.Result,
		ObservationReason:   observationRun.ReasonCode,
		EvaluatedProfiles:   eval.EvaluatedProfiles,
		ProfileState:        profileDetail.State,
		QueuedEvalRunState:  queuedEvaluationRun.State,
		QueuedEvalRunSource: queuedEvaluationRun.TriggerSource,
		RequestLogResult:    requestLog["result"].(string),
		RequestLogState:     requestLog["state"].(string),
		RequestLogSuccess:   success,
	}
}
