package sqlite

import (
	"context"
	"testing"

	appsubscriptions "proxygateway/internal/application/subscriptions"
)

func TestSubscriptionRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := NewSubscriptionRepository(db)

	mustExec(t, db, `INSERT INTO subscriptions (id, name, source_type, url, content, imported_nodes, skipped_entries, skipped_summary_json, auto_refresh_enabled, auto_refresh_interval_seconds, created_at, updated_at) VALUES ('sub_1', 'Remote', 'remote', 'https://example.test/sub', '', 2, 1, '[{"reason":"malformed_entry","count":1}]', 1, 600, 100, 200)`)
	mustExec(t, db, `INSERT INTO subscriptions (id, name, source_type, content, imported_nodes, created_at, updated_at) VALUES ('sub_2', 'Local', 'local', 'direct', 1, 101, 201)`)

	list, err := repo.List(ctx, appsubscriptions.ListFilter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 || len(list.Items) != 1 || list.Items[0].ID != "sub_1" || !list.Items[0].AutoRefreshEnabled || list.Items[0].AutoRefreshIntervalSeconds != 600 {
		t.Fatalf("list = %#v", list)
	}

	loaded, found, err := repo.Load(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Name != "Remote" || loaded.ImportedNodes != 2 || loaded.SkippedEntries != 1 || loaded.UpdatedAt != 200 {
		t.Fatalf("loaded = %#v found=%t", loaded, found)
	}
	if _, found, err := repo.Load(ctx, "missing"); err != nil || found {
		t.Fatalf("missing load found=%t err=%v", found, err)
	}

	importResult, found, err := repo.LoadImportResult(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || importResult.ID != "sub_1" || importResult.ImportedNodes != 2 || importResult.SkippedEntries != 1 {
		t.Fatalf("importResult = %#v found=%t", importResult, found)
	}

	disabled := false
	interval := 0
	updated, err := repo.UpdateAutoRefresh(ctx, "sub_1", &disabled, &interval, 300)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("expected sub_1 update")
	}
	loaded, found, err = repo.Load(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.AutoRefreshEnabled || loaded.AutoRefreshIntervalSeconds != 0 || loaded.UpdatedAt != 300 {
		t.Fatalf("loaded after update = %#v found=%t", loaded, found)
	}

	updated, err = repo.UpdateAutoRefresh(ctx, "missing", &disabled, nil, 400)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("missing subscription should not update")
	}

	if err := repo.StoreRefreshError(ctx, "sub_1", "fetch failed", 500); err != nil {
		t.Fatal(err)
	}
	loaded, found, err = repo.Load(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.LastError != "fetch failed" || loaded.UpdatedAt != 500 {
		t.Fatalf("loaded after error = %#v found=%t", loaded, found)
	}
}

func TestSubscriptionTransactionRepositoriesContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := NewSubscriptionRepository(db)

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	importRepo := NewSubscriptionImportRepositoryTx(tx)
	if err := importRepo.CreateImport(appsubscriptions.ImportRecord{
		ID:                         "sub_1",
		Name:                       "Remote",
		SourceType:                 "remote",
		URL:                        "https://example.test/sub",
		Content:                    "old",
		ImportedNodes:              1,
		SkippedEntries:             0,
		SkippedSummaryJSON:         "[]",
		AutoRefreshEnabled:         true,
		AutoRefreshIntervalSeconds: 600,
		NowMillis:                  1000,
	}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := repo.Load(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Content != "old" || loaded.ImportedNodes != 1 || loaded.UpdatedAt != 1000 {
		t.Fatalf("created subscription = %#v found=%t", loaded, found)
	}

	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := NewSubscriptionImportRepositoryTx(tx).RefreshImport(appsubscriptions.ImportRecord{
		ID:                 "sub_1",
		Content:            "new",
		ImportedNodes:      2,
		SkippedEntries:     1,
		SkippedSummaryJSON: `[{"reason":"malformed_entry","count":1}]`,
		NowMillis:          2000,
	}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	importResult, found, err := repo.LoadImportResult(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || importResult.ImportedNodes != 2 || importResult.SkippedEntries != 1 {
		t.Fatalf("importResult = %#v found=%t", importResult, found)
	}

	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, created_at) VALUES ('node_stale', 'fp_stale', 'stale', 'direct', 100)`)
	mustExec(t, db, `INSERT INTO node_sources (node_id, source_id, source_name, source_type, display_name, created_at) VALUES ('node_stale', 'sub_1', 'Remote', 'subscription', 'stale', 100)`)
	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, current_node_id, node_sticky_enabled, state, config_version, created_at) VALUES ('profile_sticky', 'sticky', 'Sticky', 'fastest', 'node_stale', 1, 'ready', 7, 100)`)
	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, node_source_mode, source_ids_json, state, created_at) VALUES ('profile_specific', 'specific', 'Specific', 'fastest', 'specific_subscriptions', '["sub_1"]', 'ready', 101)`)

	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	sourceRepo := NewSubscriptionSourceRepositoryTx(tx, 3000)
	nodeIDs, err := sourceRepo.ExistingSourceNodeIDs("sub_1")
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if len(nodeIDs) != 1 || nodeIDs[0] != "node_stale" {
		_ = tx.Rollback()
		t.Fatalf("nodeIDs = %#v", nodeIDs)
	}
	refs, err := sourceRepo.RetainStickyProfilesForRemovedNode("node_stale")
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].ID != "profile_sticky" || refs[0].ConfigVersion != 7 {
		_ = tx.Rollback()
		t.Fatalf("refs = %#v", refs)
	}
	if err := sourceRepo.DeleteSubscriptionNodeSource("node_stale", "sub_1"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	retainedRefs, err := sourceRepo.RetainedStickyProfilesForRefresh()
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if len(retainedRefs) != 1 || retainedRefs[0].ID != "profile_sticky" {
		_ = tx.Rollback()
		t.Fatalf("retainedRefs = %#v", retainedRefs)
	}
	if err := sourceRepo.InvalidateProfilesForDeletedSubscription("sub_1"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	affected, err := sourceRepo.DeleteSubscription("sub_1")
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if affected != 1 {
		_ = tx.Rollback()
		t.Fatalf("affected = %d, want 1", affected)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var retainedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM retained_profile_nodes WHERE profile_id = 'profile_sticky' AND node_id = 'node_stale'`).Scan(&retainedCount); err != nil {
		t.Fatal(err)
	}
	if retainedCount != 1 {
		t.Fatalf("retainedCount = %d, want 1", retainedCount)
	}
	var stickyState, specificState string
	if err := db.QueryRow(`SELECT state FROM access_profiles WHERE id = 'profile_sticky'`).Scan(&stickyState); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT state FROM access_profiles WHERE id = 'profile_specific'`).Scan(&specificState); err != nil {
		t.Fatal(err)
	}
	if stickyState != "degraded" || specificState != "invalid_config" {
		t.Fatalf("states = sticky %q specific %q", stickyState, specificState)
	}
}
