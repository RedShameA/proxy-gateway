package app_test

import (
	"net/http/httptest"
	"testing"

	"proxygateway/internal/app"
)

func TestManualNodeImportEnqueuesObservationRun(t *testing.T) {
	t.Parallel()

	gw := app.NewForTest(t)
	srv := httptest.NewServer(gw.Handler())
	t.Cleanup(srv.Close)
	adminToken := setupAdmin(t, srv.URL)

	var imported struct {
		ID string `json:"id"`
	}
	decodeOK(t, postJSON(t, srv.URL+"/api/nodes", adminToken, map[string]any{
		"import_text": "http://127.0.0.1:19080#manual-import-observe",
	}), &imported)

	var runs struct {
		Items []map[string]any `json:"items"`
	}
	getJSON(t, srv.URL+"/api/maintenance/runs?run_type=node_observation&target_id="+imported.ID, adminToken, &runs)
	if len(runs.Items) == 0 {
		t.Fatal("expected node observation run after manual import")
	}
	found := false
	for _, run := range runs.Items {
		if run["trigger_source"] == "manual_node_import" && run["target_id"] == imported.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("unexpected observation runs after manual import: %#v", runs.Items)
	}
}
