package sqlite

import (
	"context"
	"testing"

	appevaluations "proxygateway/internal/application/evaluations"
)

func TestEvaluationRepositoryLoadsTargetsAndUpdatesState(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO access_profiles (
		id, profile_identifier, name, type, fixed_node_id, exit_node_ids_json, chain_evaluation_mode, test_url,
		egress_country_mode, egress_countries_json, node_source_mode, source_ids_json, protocols_json,
		candidate_limit, min_evaluation_interval_seconds, last_evaluated_at, config_version,
		relative_improvement_threshold, absolute_latency_improvement_ms, node_sticky_enabled, created_at
	) VALUES (
		'profile_1', 'profile', 'Profile', 'chain', 'node_exit', '["node_exit"]', 'chain_link', 'example.test',
		'include', '["US"]', 'specific_subscriptions', '["sub_1"]', '["vmess"]',
		7, 30, 1000, 3, 0.25, 50, 1, 100
	)`)

	repo := NewEvaluationRepository(db)
	targets, err := repo.ListTargets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %#v", targets)
	}
	target := targets[0]
	if target.ID != "profile_1" || target.Type != "chain" || target.ExitNodeIDs[0] != "node_exit" || !target.NodeStickyEnabled {
		t.Fatalf("target = %#v", target)
	}

	updated, err := repo.UpdateProfileState(context.Background(), "profile_1", 2, appevaluations.StateUpdate{
		State: evalStateString("ready"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("updated with stale config version = true, want false")
	}

	updated, err = repo.UpdateProfileState(context.Background(), "profile_1", 3, appevaluations.StateUpdate{
		State:                          evalStateString("ready"),
		LastError:                      evalStateString(""),
		CurrentNodeID:                  evalStateString("node_front"),
		CurrentExitNodeID:              evalStateString("node_exit"),
		CurrentPathLatencyMS:           evalStateInt64(88),
		CurrentPathFailedEvaluations:   evalStateInt(0),
		CurrentPathMissedSuccessCycles: evalStateInt(0),
		SwitchReason:                   evalStateString("candidate_clearly_better"),
		LastEvaluationDetailsJSON:      evalStateString(`{"selected_node_id":"node_front"}`),
		LastEvaluatedAt:                evalStateInt64(2000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("updated = false, want true")
	}
	path, err := repo.CurrentChainPath(context.Background(), "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if path.FrontNodeID != "node_front" || path.ExitNodeID != "node_exit" {
		t.Fatalf("path = %#v", path)
	}
}

func TestEvaluationRepositoryReleasesRetainedNodesInStateTransaction(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, config_version, node_sticky_enabled, created_at) VALUES ('profile_1', 'profile', 'Profile', 'fastest', 5, 1, 100)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, created_at) VALUES ('node_keep', 'fp_keep', 'Keep', 'direct', 100)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, created_at) VALUES ('node_release', 'fp_release', 'Release', 'direct', 101)`)
	mustExec(t, db, `INSERT INTO retained_profile_nodes (profile_id, node_id, created_at) VALUES ('profile_1', 'node_keep', 100)`)
	mustExec(t, db, `INSERT INTO retained_profile_nodes (profile_id, node_id, created_at) VALUES ('profile_1', 'node_release', 101)`)
	mustExec(t, db, `INSERT INTO node_observations (node_id, usable) VALUES ('node_release', 1)`)

	repo := NewEvaluationRepository(db)
	result, err := repo.UpdateProfileStateAndReleaseRetained(context.Background(), "profile_1", 5, []string{"node_keep"}, true, appevaluations.StateUpdate{
		State:           evalStateString("ready"),
		CurrentNodeID:   evalStateString("node_keep"),
		LastEvaluatedAt: evalStateInt64(3000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp_release" {
		t.Fatalf("DeletedFingerprints = %#v", result.DeletedFingerprints)
	}
	for _, item := range []struct {
		name  string
		query string
		want  int
	}{
		{name: "kept retained", query: `SELECT COUNT(*) FROM retained_profile_nodes WHERE profile_id = 'profile_1' AND node_id = 'node_keep'`, want: 1},
		{name: "released retained", query: `SELECT COUNT(*) FROM retained_profile_nodes WHERE profile_id = 'profile_1' AND node_id = 'node_release'`, want: 0},
		{name: "released node", query: `SELECT COUNT(*) FROM nodes WHERE id = 'node_release'`, want: 0},
		{name: "released observation", query: `SELECT COUNT(*) FROM node_observations WHERE node_id = 'node_release'`, want: 0},
	} {
		var got int
		if err := db.QueryRow(item.query).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != item.want {
			t.Fatalf("%s count = %d, want %d", item.name, got, item.want)
		}
	}
}

func evalStateString(value string) *string {
	return &value
}

func evalStateInt(value int) *int {
	return &value
}

func evalStateInt64(value int64) *int64 {
	return &value
}
