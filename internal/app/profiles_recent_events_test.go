package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNodeObservationMaintenanceDoesNotEnqueueProfileEvaluations(t *testing.T) {
	t.Parallel()

	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ip=203.0.113.45\nloc=US\n"))
	}))
	t.Cleanup(probe.Close)

	g := NewForTest(t)
	if _, err := g.db.Exec(
		`INSERT INTO nodes (id, fingerprint, name, type, created_at) VALUES ('node_observed', 'fp-observed', 'observed', 'direct', ?)`,
		unixMillisNow(),
	); err != nil {
		t.Fatal(err)
	}
	cfg := defaultAccessProfileConfig("profile_auto")
	cfg.Name = "auto"
	cfg.Type = "fastest"
	cfg.AutoEvalEnabled = true
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}

	run, err := g.createNodeObservationRun("manual", "single_node", []nodeRecord{{ID: "node_observed", Name: "observed", Enabled: true}}, probe.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = g.runNodeObservationMaintenanceRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}

	var count int
	if err := g.db.QueryRow(`SELECT COUNT(*) FROM maintenance_runs WHERE run_type = 'profile_evaluation' AND target_id = ?`, cfg.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("profile evaluation runs after node observation = %d, want 0", count)
	}
}

func TestAccessProfileRecentEventsExposeSkippedEvaluationAndTriggerSource(t *testing.T) {
	t.Parallel()

	g := NewForTest(t)
	cfg := defaultAccessProfileConfig("profile_events")
	cfg.Name = "events"
	cfg.Type = "chain"
	if err := g.insertAccessProfileConfig(cfg); err != nil {
		t.Fatal(err)
	}
	skipped, err := g.createMaintenanceRun("profile_evaluation", "node_observation", cfg.ID, cfg.Name, 0, map[string]any{"config_version": cfg.ConfigVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.finishMaintenanceRun(skipped.ID, maintenanceRunResultSkipped, "min_interval_not_reached", 0, skipped.detail(), ""); err != nil {
		t.Fatal(err)
	}
	failed, err := g.createMaintenanceRun("profile_evaluation", "access_profile_change", cfg.ID, cfg.Name, 1, map[string]any{"config_version": cfg.ConfigVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.finishMaintenanceRun(failed.ID, maintenanceRunResultFailure, "evaluation_failed", 1, failed.detail(), "EOF"); err != nil {
		t.Fatal(err)
	}
	now := unixMillisNow()
	if _, err := g.db.Exec(`UPDATE maintenance_runs SET created_at = CASE id WHEN ? THEN ? WHEN ? THEN ? ELSE created_at END`, skipped.ID, now, failed.ID, now+1); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/access-profiles/"+cfg.ID, nil)
	g.handleAccessProfileGet(rec, req, cfg.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		RecentEvents []struct {
			RunType       string `json:"run_type"`
			TriggerSource string `json:"trigger_source"`
			Result        string `json:"result"`
			ReasonCode    string `json:"reason_code"`
			LastError     string `json:"last_error"`
		} `json:"recent_events"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.RecentEvents) != 2 {
		t.Fatalf("recent_events = %#v, want two events", body.RecentEvents)
	}
	failedEvent := body.RecentEvents[0]
	if failedEvent.RunType != "profile_evaluation" {
		t.Fatalf("failed event run_type = %q, want profile_evaluation", failedEvent.RunType)
	}
	if failedEvent.TriggerSource != "access_profile_change" {
		t.Fatalf("failed event trigger = %q, want access_profile_change", failedEvent.TriggerSource)
	}
	if failedEvent.Result != "failure" || failedEvent.ReasonCode != "evaluation_failed" || failedEvent.LastError != "EOF" {
		t.Fatalf("failed event status = %#v, want failure evaluation_failed EOF", failedEvent)
	}
	skippedEvent := body.RecentEvents[1]
	if skippedEvent.RunType != "profile_evaluation" {
		t.Fatalf("skipped event run_type = %q, want profile_evaluation", skippedEvent.RunType)
	}
	if skippedEvent.TriggerSource != "node_observation" {
		t.Fatalf("skipped event trigger = %q, want node_observation", skippedEvent.TriggerSource)
	}
	if skippedEvent.Result != "skipped" || skippedEvent.ReasonCode != "min_interval_not_reached" {
		t.Fatalf("skipped event status = %#v, want skipped min_interval_not_reached", skippedEvent)
	}
}
