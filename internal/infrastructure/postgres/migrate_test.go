package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const testPostgresDSNEnv = "PROXYGATEWAY_TEST_POSTGRES_DSN"

func TestMigrateCreatesPostgresBaselineSchema(t *testing.T) {
	db, cleanup := openIsolatedPostgresTestDB(t)
	defer cleanup()

	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	assertPostgresColumn(t, db, "nodes", "fingerprint", "text", false, "")
	assertPostgresColumn(t, db, "nodes", "enabled", "boolean", false, "true")
	assertPostgresColumn(t, db, "access_profiles", "manual_only", "boolean", false, "false")
	assertPostgresColumn(t, db, "access_profiles", "last_evaluation_details_json", "text", false, "'{}'::text")
	assertPostgresColumn(t, db, "maintenance_runs", "sequence", "bigint", false, "nextval")
	assertPostgresColumn(t, db, "request_logs", "success", "boolean", true, "")
	assertPostgresConstraint(t, db, "nodes", "nodes_fingerprint_key", "UNIQUE")
	assertPostgresConstraint(t, db, "node_sources", "node_sources_pkey", "PRIMARY KEY")
	assertPostgresConstraint(t, db, "retained_profile_nodes", "retained_profile_nodes_pkey", "PRIMARY KEY")
	assertPostgresConstraint(t, db, "node_observations", "node_observations_pkey", "PRIMARY KEY")
	assertPostgresConstraint(t, db, "geoip_status", "geoip_status_pkey", "PRIMARY KEY")
	assertPostgresConstraint(t, db, "kv_settings", "kv_settings_pkey", "PRIMARY KEY")
	assertPostgresConstraint(t, db, "proxy_credentials", "fk_proxy_credentials_profile", "FOREIGN KEY")
	assertPostgresHasNoForeignKeys(t, db, "request_logs")
	assertPostgresIndex(t, db, "idx_nodes_created_id")
	assertPostgresIndex(t, db, "idx_nodes_enabled_created")
	assertPostgresIndex(t, db, "idx_access_profiles_created_id")
	assertPostgresIndex(t, db, "idx_access_profiles_identifier")
	assertPostgresIndex(t, db, "idx_maintenance_runs_type_state_created")
	assertPostgresIndex(t, db, "idx_request_logs_ts")
	assertPostgresIndex(t, db, "idx_request_logs_state_success_ts")
	assertPostgresIndex(t, db, "idx_request_logs_profile_ts")
	assertPostgresIndex(t, db, "idx_request_logs_credential_ts")
	assertPostgresIndex(t, db, "idx_request_logs_target_host")
	assertPostgresIndex(t, db, "idx_request_logs_proxy_path_json")
	assertPostgresIndex(t, db, "idx_proxy_credentials_lookup")
	assertPostgresIndex(t, db, "idx_subscriptions_auto_refresh")
	assertPostgresTableExists(t, db, "goose_db_version")
	assertPostgresGooseVersion(t, db, initialSchemaMigrationVersion)
}

func TestMigratePostgresConflictRollsBackBaselineTables(t *testing.T) {
	db, cleanup := openIsolatedPostgresTestDB(t)
	defer cleanup()

	if _, err := db.ExecContext(context.Background(), `CREATE TABLE nodes (id text PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err == nil {
		t.Fatal("expected migration to fail on conflicting schema")
	}
	assertPostgresTableMissing(t, db, "admin_credentials")
	assertPostgresTableMissing(t, db, "maintenance_runs")
}

func openIsolatedPostgresTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv(testPostgresDSNEnv))
	if dsn == "" {
		t.Skipf("%s is not set", testPostgresDSNEnv)
	}
	base, err := sql.Open(DriverName, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = base.Close() })

	schema := fmt.Sprintf("proxygateway_test_%d", time.Now().UnixNano())
	if _, err := base.ExecContext(context.Background(), `CREATE SCHEMA `+quoteIdent(schema)); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		_, _ = base.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+quoteIdent(schema)+` CASCADE`)
	}
	t.Cleanup(cleanup)

	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.RuntimeParams == nil {
		config.RuntimeParams = map[string]string{}
	}
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	ConfigureConnection(db)
	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db, func() { _ = db.Close() }
}

func assertPostgresColumn(t *testing.T, db *sql.DB, tableName, columnName, dataType string, nullable bool, defaultContains string) {
	t.Helper()

	var gotDataType, gotNullable string
	var gotDefault sql.NullString
	err := db.QueryRowContext(
		context.Background(),
		`SELECT data_type, is_nullable, column_default
		   FROM information_schema.columns
		  WHERE table_schema = current_schema()
		    AND table_name = $1
		    AND column_name = $2`,
		tableName,
		columnName,
	).Scan(&gotDataType, &gotNullable, &gotDefault)
	if err != nil {
		t.Fatalf("load %s.%s column: %v", tableName, columnName, err)
	}
	if gotDataType != dataType {
		t.Fatalf("%s.%s data_type = %q, want %q", tableName, columnName, gotDataType, dataType)
	}
	wantNullable := "NO"
	if nullable {
		wantNullable = "YES"
	}
	if gotNullable != wantNullable {
		t.Fatalf("%s.%s nullable = %q, want %q", tableName, columnName, gotNullable, wantNullable)
	}
	if defaultContains != "" {
		if !gotDefault.Valid || !strings.Contains(gotDefault.String, defaultContains) {
			t.Fatalf("%s.%s default = %q, want contains %q", tableName, columnName, gotDefault.String, defaultContains)
		}
	}
}

func assertPostgresConstraint(t *testing.T, db *sql.DB, tableName, constraintName, constraintType string) {
	t.Helper()

	var found string
	err := db.QueryRowContext(
		context.Background(),
		`SELECT constraint_type
		   FROM information_schema.table_constraints
		  WHERE table_schema = current_schema()
		    AND table_name = $1
		    AND constraint_name = $2`,
		tableName,
		constraintName,
	).Scan(&found)
	if err != nil {
		t.Fatalf("constraint %s on %s missing: %v", constraintName, tableName, err)
	}
	if found != constraintType {
		t.Fatalf("constraint %s type = %q, want %q", constraintName, found, constraintType)
	}
}

func assertPostgresIndex(t *testing.T, db *sql.DB, indexName string) {
	t.Helper()

	var exists bool
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT EXISTS (
			SELECT 1
			  FROM pg_indexes
			 WHERE schemaname = current_schema()
			   AND indexname = $1
		)`,
		indexName,
	).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("index %s is missing", indexName)
	}
}

func assertPostgresHasNoForeignKeys(t *testing.T, db *sql.DB, tableName string) {
	t.Helper()

	var count int
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT COUNT(*)
		   FROM information_schema.table_constraints
		  WHERE table_schema = current_schema()
		    AND table_name = $1
		    AND constraint_type = 'FOREIGN KEY'`,
		tableName,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("table %s has %d foreign keys, want none", tableName, count)
	}
}

func assertPostgresGooseVersion(t *testing.T, db *sql.DB, version int64) {
	t.Helper()

	var exists bool
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT EXISTS (
			SELECT 1
			  FROM goose_db_version
			 WHERE version_id = $1
			   AND is_applied = true
		)`,
		version,
	).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("goose version %d is missing", version)
	}
}

func assertPostgresTableExists(t *testing.T, db *sql.DB, tableName string) {
	t.Helper()
	if !postgresTableExists(t, db, tableName) {
		t.Fatalf("table %s is missing", tableName)
	}
}

func assertPostgresTableMissing(t *testing.T, db *sql.DB, tableName string) {
	t.Helper()
	if postgresTableExists(t, db, tableName) {
		t.Fatalf("table %s exists after failed migration", tableName)
	}
}

func postgresTableExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()
	var exists bool
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT EXISTS (
			SELECT 1
			  FROM information_schema.tables
			 WHERE table_schema = current_schema()
			   AND table_name = $1
		)`,
		tableName,
	).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	return exists
}

func quoteIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
