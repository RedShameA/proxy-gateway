package sqlite

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

func TestOverviewRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := NewOverviewRepository(db)

	mustExec(t, db, `INSERT INTO subscriptions (id, name, source_type, created_at, updated_at) VALUES ('sub_1', 'sub 1', 'remote', 100, 100)`)
	mustExec(t, db, `INSERT INTO subscriptions (id, name, source_type, created_at, updated_at) VALUES ('sub_2', 'sub 2', 'local', 101, 101)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_1', 'fp_1', 'node 1', 'direct', 1, 100)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_2', 'fp_2', 'node 2', 'direct', 0, 101)`)
	mustExec(t, db, `INSERT INTO node_observations (node_id, usable, egress_country) VALUES ('node_1', 1, 'US')`)
	mustExec(t, db, `INSERT INTO node_observations (node_id, usable, egress_country) VALUES ('node_2', 1, 'JP')`)
	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, state, created_at) VALUES ('profile_1', 'p1', 'profile 1', 'fixed', 'ready', 100)`)
	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, state, created_at) VALUES ('profile_2', 'p2', 'profile 2', 'fastest', 'failed', 101)`)
	mustExec(t, db, `INSERT INTO proxy_credentials (id, profile_id, remark, password, password_hash, created_at) VALUES ('cred_1', 'profile_1', 'client', 'secret', 'hash', 100)`)

	counts, err := repo.LoadResourceCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Subscriptions != 2 || counts.Nodes != 2 || counts.UsableNodes != 1 || counts.Profiles != 2 || counts.Credentials != 1 {
		t.Fatalf("counts = %#v", counts)
	}

	stateCounts, err := repo.LoadProfileStateCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stateCounts["ready"] != 1 || stateCounts["failed"] != 1 {
		t.Fatalf("stateCounts = %#v", stateCounts)
	}
}

func TestDictionaryRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	repo := NewDictionaryRepository(db)

	mustExec(t, db, `INSERT INTO node_observations (node_id, egress_country) VALUES ('node_1', 'US')`)
	mustExec(t, db, `INSERT INTO node_observations (node_id, egress_country) VALUES ('node_2', 'JP')`)
	mustExec(t, db, `INSERT INTO node_observations (node_id, egress_country) VALUES ('node_3', 'US')`)
	mustExec(t, db, `INSERT INTO node_observations (node_id, egress_country) VALUES ('node_4', '')`)

	countries, err := repo.ListEgressCountries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"JP", "US"}; !reflect.DeepEqual(countries, want) {
		t.Fatalf("countries = %#v, want %#v", countries, want)
	}
}

func TestProxyCredentialRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := NewProxyCredentialRepository(db)

	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, created_at) VALUES ('profile_1', 'work', 'Work', 'fixed', 100)`)
	mustExec(t, db, `INSERT INTO proxy_credentials (id, profile_id, remark, password, password_hash, enabled, created_at, last_used_at) VALUES ('cred_1', 'profile_1', 'client', 'secret', 'hash', 1, 100, 1000)`)
	mustExec(t, db, `INSERT INTO proxy_credentials (id, profile_id, remark, password, password_hash, enabled, created_at, last_used_at) VALUES ('cred_disabled', 'profile_1', 'disabled', 'disabled', 'hash', 0, 100, 0)`)

	record, profileFound, credentialFound, err := repo.LookupCredential(ctx, "work", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !profileFound || !credentialFound || record.ID != "cred_1" || record.Remark != "client" || record.ProfileID != "profile_1" {
		t.Fatalf("lookup success = %#v profileFound=%t credentialFound=%t", record, profileFound, credentialFound)
	}
	if _, profileFound, credentialFound, err := repo.LookupCredential(ctx, "missing", "secret"); err != nil || profileFound || credentialFound {
		t.Fatalf("missing profile: profileFound=%t credentialFound=%t err=%v", profileFound, credentialFound, err)
	}
	if _, profileFound, credentialFound, err := repo.LookupCredential(ctx, "work", "disabled"); err != nil || !profileFound || credentialFound {
		t.Fatalf("disabled credential: profileFound=%t credentialFound=%t err=%v", profileFound, credentialFound, err)
	}
	if err := repo.TouchCredentialLastUsed(ctx, "cred_1", 2000, 1500); err != nil {
		t.Fatal(err)
	}
	var lastUsed int64
	if err := db.QueryRow(`SELECT last_used_at FROM proxy_credentials WHERE id = 'cred_1'`).Scan(&lastUsed); err != nil {
		t.Fatal(err)
	}
	if lastUsed != 2000 {
		t.Fatalf("lastUsed = %d, want 2000", lastUsed)
	}
	if err := repo.TouchCredentialLastUsed(ctx, "cred_1", 3000, 1500); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT last_used_at FROM proxy_credentials WHERE id = 'cred_1'`).Scan(&lastUsed); err != nil {
		t.Fatal(err)
	}
	if lastUsed != 2000 {
		t.Fatalf("recent lastUsed = %d, want unchanged 2000", lastUsed)
	}
}

func TestMaintenanceAuxiliaryRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := NewMaintenanceAuxiliaryRepository(db)

	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_sourced', 'fp_sourced', 'sourced', 'direct', 1, 100)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_disabled', 'fp_disabled', 'disabled', 'direct', 0, 101)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_retained', 'fp_retained', 'retained', 'direct', 1, 102)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_manual', 'fp_manual', 'manual', 'direct', 1, 103)`)
	mustExec(t, db, `INSERT INTO node_sources (node_id, source_id, source_name, source_type, display_name, created_at) VALUES ('node_sourced', 'sub_1', 'sub', 'subscription', 'sourced', 100)`)
	mustExec(t, db, `INSERT INTO retained_profile_nodes (profile_id, node_id, created_at) VALUES ('profile_retained', 'node_retained', 100)`)

	nodeTargets, err := repo.ListNodeObservationScheduleTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodeTargets) != 2 || nodeTargets[0].ID != "node_sourced" || nodeTargets[1].ID != "node_manual" {
		t.Fatalf("nodeTargets = %#v", nodeTargets)
	}

	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, last_evaluated_at, auto_evaluation_enabled, auto_evaluation_interval_seconds, config_version, created_at) VALUES ('profile_fast', 'fast', 'Fast', 'fastest', 0, 1, 0, 7, 100)`)
	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, last_evaluated_at, auto_evaluation_enabled, auto_evaluation_interval_seconds, config_version, created_at) VALUES ('profile_chain', 'chain', 'Chain', 'chain', 1200, 0, 300, 8, 101)`)
	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, created_at) VALUES ('profile_fixed', 'fixed', 'Fixed', 'fixed', 102)`)

	profileTargets, err := repo.ListProfileEvaluationScheduleTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(profileTargets) != 2 || profileTargets[0].ID != "profile_fast" || !profileTargets[0].AutoEvaluationEnabled || profileTargets[0].ConfigVersion != 7 {
		t.Fatalf("profileTargets = %#v", profileTargets)
	}
	if profileTargets[1].ID != "profile_chain" || profileTargets[1].AutoEvaluationEnabled || profileTargets[1].AutoEvaluationIntervalSeconds != 300 {
		t.Fatalf("profileTargets = %#v", profileTargets)
	}

	mustExec(t, db, `INSERT INTO subscriptions (id, name, source_type, auto_refresh_enabled, auto_refresh_interval_seconds, created_at, updated_at) VALUES ('sub_remote', 'Remote', 'remote', 1, 600, 100, 200)`)
	mustExec(t, db, `INSERT INTO subscriptions (id, name, source_type, auto_refresh_enabled, auto_refresh_interval_seconds, created_at, updated_at) VALUES ('sub_disabled', 'Disabled', 'remote', 0, 0, 101, 201)`)
	mustExec(t, db, `INSERT INTO subscriptions (id, name, source_type, created_at, updated_at) VALUES ('sub_local', 'Local', 'local', 102, 202)`)

	subscriptionTargets, err := repo.ListSubscriptionRefreshScheduleTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptionTargets) != 2 || subscriptionTargets[0].ID != "sub_remote" || !subscriptionTargets[0].AutoRefreshEnabled || subscriptionTargets[0].AutoRefreshIntervalSeconds != 600 {
		t.Fatalf("subscriptionTargets = %#v", subscriptionTargets)
	}
	if subscriptionTargets[1].ID != "sub_disabled" || subscriptionTargets[1].AutoRefreshEnabled {
		t.Fatalf("subscriptionTargets = %#v", subscriptionTargets)
	}

	mustExec(t, db, `INSERT INTO maintenance_runs (id, run_type, trigger_source, state, created_at, updated_at) VALUES ('run_old', 'log_cleanup', 'scheduled', 'finished', 100, 100)`)
	mustExec(t, db, `INSERT INTO maintenance_runs (id, run_type, trigger_source, state, created_at, updated_at) VALUES ('run_keep', 'log_cleanup', 'scheduled', 'finished', 200, 200)`)
	mustExec(t, db, `INSERT INTO maintenance_runs (id, run_type, trigger_source, state, created_at, updated_at) VALUES ('run_recent', 'log_cleanup', 'scheduled', 'finished', 5000, 5000)`)
	mustExec(t, db, `INSERT INTO maintenance_runs (id, run_type, trigger_source, state, created_at, updated_at) VALUES ('run_other', 'geoip_update', 'scheduled', 'finished', 6000, 6000)`)

	recent, err := repo.HasRecentRun(ctx, "log_cleanup", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !recent {
		t.Fatal("expected recent log cleanup run")
	}
	recent, err = repo.HasRecentRun(ctx, "subscription_refresh", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if recent {
		t.Fatal("did not expect recent subscription refresh run")
	}

	deleted, err := repo.DeleteHistoryBefore(ctx, 1000, "run_keep")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM maintenance_runs WHERE id IN ('run_old', 'run_keep', 'run_recent')`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("remaining = %d, want 2", remaining)
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
