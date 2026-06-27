package sqlite

import (
	"context"
	"testing"

	appprofiles "proxygateway/internal/application/profiles"
)

func TestProfileConfigRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := NewProfileConfigRepository(db)

	record := appprofiles.ConfigRecord{
		ID:                           "profile_1",
		Name:                         "Fast",
		ProfileIdentifier:            "fast",
		Type:                         "fastest",
		TestURL:                      "https://example.test/204",
		EgressCountry:                "US",
		EgressCountryMode:            "include",
		EgressCountries:              []string{"US", "JP"},
		NodeSourceMode:               "specific_subscriptions",
		SourceIDs:                    []string{"sub_1"},
		Protocols:                    []string{"direct"},
		NameIncludeRegex:             "tokyo",
		MinEvaluationIntervalSeconds: 10,
		CandidateLimit:               20,
		RelativeImprovementThreshold: 0.3,
		AbsoluteLatencyImprovementMS: 50,
		CurrentNodeID:                "node_1",
		State:                        "ready",
		AutoEvaluationEnabled:        true,
		AutoEvaluationInterval:       60,
		NodeStickyEnabled:            true,
		ConfigVersion:                3,
	}
	if err := repo.CreateConfig(ctx, record, 1000); err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListConfigIDs(ctx, appprofiles.ListConfigFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.IDs) != 1 || list.IDs[0] != "profile_1" {
		t.Fatalf("list = %#v", list)
	}
	loaded, found, err := repo.LoadConfig(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.ProfileIdentifier != "fast" || len(loaded.EgressCountries) != 2 || loaded.SourceIDs[0] != "sub_1" || !loaded.AutoEvaluationEnabled {
		t.Fatalf("loaded = %#v found=%t", loaded, found)
	}
	duplicate, err := repo.ProfileIdentifierExists(ctx, "fast", "profile_other")
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate {
		t.Fatal("expected duplicate identifier")
	}
	exists, err := repo.Exists(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected profile to exist")
	}

	mustExec(t, db, `UPDATE access_profiles SET last_error = 'old error', current_path_failed_evaluations = 2, current_path_missed_success_cycles = 3, switch_reason = 'old', last_evaluation_details_json = '{"old":true}', last_evaluated_at = 900, last_evaluation_started_at = 800 WHERE id = 'profile_1'`)
	record.Name = "Fast Updated"
	record.SwitchReason = "config_updated"
	record.LastEvaluationDetailsJSON = "{}"
	record.ConfigVersion = 4
	if err := repo.UpdateConfig(ctx, record, appprofiles.ConfigUpdateOptions{EvaluationChanged: true, ResetCurrentPath: true}); err != nil {
		t.Fatal(err)
	}
	loaded, found, err = repo.LoadConfig(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Name != "Fast Updated" || loaded.ConfigVersion != 4 || loaded.LastError != "" || loaded.SwitchReason != "config_updated" || loaded.LastEvaluatedAt != 0 {
		t.Fatalf("loaded after update = %#v found=%t", loaded, found)
	}
	var failures, missed int
	if err := db.QueryRow(`SELECT current_path_failed_evaluations, current_path_missed_success_cycles FROM access_profiles WHERE id = 'profile_1'`).Scan(&failures, &missed); err != nil {
		t.Fatal(err)
	}
	if failures != 0 || missed != 0 {
		t.Fatalf("path counters = %d/%d, want 0/0", failures, missed)
	}
}
