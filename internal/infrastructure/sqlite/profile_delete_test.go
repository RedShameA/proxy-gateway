package sqlite

import (
	"context"
	"testing"
)

func TestProfileDeleteRepositoryTxContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	mustExec(t, db, `INSERT INTO access_profiles (id, profile_identifier, name, type, created_at) VALUES ('profile_1', 'profile', 'Profile', 'fastest', 100)`)
	mustExec(t, db, `INSERT INTO proxy_credentials (id, profile_id, remark, password, password_hash, created_at) VALUES ('cred_1', 'profile_1', 'client', 'secret', '', 100)`)
	mustExec(t, db, `INSERT INTO maintenance_runs (id, run_type, trigger_source, target_id, state, created_at, updated_at) VALUES ('run_1', 'profile_evaluation', 'manual', 'profile_1', 'finished', 100, 100)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, created_at) VALUES ('node_1', 'fp_1', 'Node', 'direct', 100)`)
	mustExec(t, db, `INSERT INTO retained_profile_nodes (profile_id, node_id, created_at) VALUES ('profile_1', 'node_1', 100)`)

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	repo := NewProfileDeleteRepositoryTx(tx)
	nodeIDs, err := repo.RetainedNodeIDs("profile_1")
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if len(nodeIDs) != 1 || nodeIDs[0] != "node_1" {
		_ = tx.Rollback()
		t.Fatalf("nodeIDs = %#v", nodeIDs)
	}
	if err := repo.DeleteCredentials("profile_1"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := repo.DeleteMaintenanceRuns("profile_1"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := repo.DeleteRetainedNodes("profile_1"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	affected, err := repo.DeleteProfile("profile_1")
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

	for _, item := range []struct {
		name  string
		query string
	}{
		{name: "profile", query: `SELECT COUNT(*) FROM access_profiles WHERE id = 'profile_1'`},
		{name: "credentials", query: `SELECT COUNT(*) FROM proxy_credentials WHERE profile_id = 'profile_1'`},
		{name: "runs", query: `SELECT COUNT(*) FROM maintenance_runs WHERE target_id = 'profile_1'`},
		{name: "retained", query: `SELECT COUNT(*) FROM retained_profile_nodes WHERE profile_id = 'profile_1'`},
	} {
		var count int
		if err := db.QueryRow(item.query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want 0", item.name, count)
		}
	}
}
