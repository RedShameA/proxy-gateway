package app

import (
	"database/sql"
	"testing"

	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
)

func TestRequestLogsMigrationRebuildsSuccessAsNullable(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE request_logs (
		id TEXT PRIMARY KEY,
		ts INTEGER NOT NULL,
		proxy_credential_id TEXT NOT NULL DEFAULT '',
		proxy_credential TEXT NOT NULL,
		access_profile_id TEXT NOT NULL DEFAULT '',
		access_profile TEXT NOT NULL,
		access_profile_identifier TEXT NOT NULL DEFAULT '',
		target_host TEXT NOT NULL,
		proxy_path TEXT NOT NULL,
		proxy_path_json TEXT NOT NULL DEFAULT '',
		success INTEGER NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		duration_ms INTEGER NOT NULL DEFAULT 0,
		ingress_bytes INTEGER NOT NULL DEFAULT 0,
		egress_bytes INTEGER NOT NULL DEFAULT 0,
		http_status INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO request_logs (
		id, ts, proxy_credential, access_profile, target_host, proxy_path, success
	) VALUES ('old_log', 1700000000000, 'client', 'profile', 'example.com:443', 'node-a', 1)`)
	if err != nil {
		t.Fatal(err)
	}
	g := &Gateway{db: db, dataDir: dataDir}
	if err := g.migrate(); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := g.db.Query(`PRAGMA table_info(request_logs)`)
	if err != nil {
		t.Fatal(err)
	}
	successAllowsNull := false
	hasState := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		if name == "success" {
			successAllowsNull = notNull == 0
		}
		if name == "state" {
			hasState = true
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if !hasState || !successAllowsNull {
		t.Fatalf("request_logs schema state=%t successAllowsNull=%t, want true/true", hasState, successAllowsNull)
	}

	var state string
	var success sql.NullInt64
	var failureStage string
	if err := g.db.QueryRow(`SELECT state, success, failure_stage FROM request_logs WHERE id = 'old_log'`).Scan(&state, &success, &failureStage); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || !success.Valid || success.Int64 != 1 || failureStage != "" {
		t.Fatalf("migrated request log state=%q success=%v failure_stage=%q, want completed/1 empty", state, success, failureStage)
	}
	assertGooseVersion(t, g.db)
}

func TestAccessProfilesMigrationDropsMaximumHoldSeconds(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE access_profiles (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		relative_improvement_threshold REAL NOT NULL DEFAULT 0.2,
		absolute_latency_improvement_ms INTEGER NOT NULL DEFAULT 100,
		maximum_hold_seconds INTEGER NOT NULL DEFAULT 300,
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE kv_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kv_settings (key, value) VALUES ('switching_tolerance.maximum_hold_seconds', '300')`); err != nil {
		t.Fatal(err)
	}
	g := &Gateway{db: db, dataDir: dataDir}
	if err := g.migrate(); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := g.db.Query(`PRAGMA table_info(access_profiles)`)
	if err != nil {
		t.Fatal(err)
	}
	hasMaximumHold := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		if name == "maximum_hold_seconds" {
			hasMaximumHold = true
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if hasMaximumHold {
		t.Fatal("maximum_hold_seconds column should be dropped")
	}

	var legacyKVCount int
	if err := g.db.QueryRow(`SELECT COUNT(*) FROM kv_settings WHERE key = 'switching_tolerance.maximum_hold_seconds'`).Scan(&legacyKVCount); err != nil {
		t.Fatal(err)
	}
	if legacyKVCount != 0 {
		t.Fatalf("legacy maximum_hold_seconds kv count = %d, want 0", legacyKVCount)
	}
	assertGooseVersion(t, g.db)
}

func TestMigrationRecordsGooseVersion(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	g := &Gateway{db: db, dataDir: dataDir}
	if err := g.migrate(); err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	assertGooseVersion(t, g.db)
}

func assertGooseVersion(t *testing.T, db *sql.DB) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM goose_db_version WHERE version_id = 202606260001 AND is_applied = 1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("goose migration version count = %d, want 1", count)
	}
}
