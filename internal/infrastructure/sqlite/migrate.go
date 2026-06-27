package sqlite

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"

	applicationnodes "proxygateway/internal/application/nodes"
	databaseinfra "proxygateway/internal/infrastructure/database"
)

const initialSchemaMigrationVersion int64 = 202606260001

func Migrate(ctx context.Context, db *sql.DB) error {
	if err := configure(db); err != nil {
		return err
	}
	if err := databaseinfra.Migrate(ctx, db, databaseinfra.MigrationSet{
		Dialect: databaseinfra.DialectSQLite,
		GoMigrations: []*goose.Migration{
			goose.NewGoMigration(
				initialSchemaMigrationVersion,
				&goose.GoFunc{RunTx: func(_ context.Context, tx *sql.Tx) error {
					return reconcileSchema(tx)
				}},
				nil,
			),
		},
	}); err != nil {
		return err
	}
	if err := closeInterruptedRequestLogs(db); err != nil {
		return err
	}
	return backfillNodeOutboundJSON(ctx, db)
}

func configure(db *sql.DB) error {
	if err := ConfigureConnection(db); err != nil {
		return err
	}
	_, err := db.Exec(`PRAGMA journal_mode = WAL`)
	return err
}

type schemaExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
}

func reconcileSchema(db schemaExecutor) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS admin_credentials (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			password_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_sessions (
			token_hash TEXT PRIMARY KEY,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			fingerprint TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			server TEXT NOT NULL DEFAULT '',
			server_port INTEGER NOT NULL DEFAULT 0,
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			raw_json TEXT NOT NULL DEFAULT '',
			outbound_json TEXT NOT NULL DEFAULT '',
			source_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS node_sources (
			node_id TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_name TEXT NOT NULL,
			source_type TEXT NOT NULL,
			display_name TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (node_id, source_id)
		)`,
		`CREATE TABLE IF NOT EXISTS retained_profile_nodes (
			profile_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			PRIMARY KEY (profile_id, node_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_retained_profile_nodes_node ON retained_profile_nodes (node_id)`,
		`CREATE TABLE IF NOT EXISTS node_observations (
			node_id TEXT PRIMARY KEY,
			usable INTEGER NOT NULL DEFAULT 0,
			egress_ip TEXT NOT NULL DEFAULT '',
			egress_country TEXT NOT NULL DEFAULT '',
			latency_ms INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			last_success_at INTEGER NOT NULL DEFAULT 0,
			last_failure_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS subscriptions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			source_type TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			imported_nodes INTEGER NOT NULL DEFAULT 0,
			skipped_entries INTEGER NOT NULL DEFAULT 0,
			skipped_summary_json TEXT NOT NULL DEFAULT '[]',
			last_error TEXT NOT NULL DEFAULT '',
			auto_refresh_enabled INTEGER NOT NULL DEFAULT 1,
			auto_refresh_interval_seconds INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS access_profiles (
			id TEXT PRIMARY KEY,
			profile_identifier TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			fixed_node_id TEXT NOT NULL DEFAULT '',
			exit_node_ids_json TEXT NOT NULL DEFAULT '[]',
			chain_evaluation_mode TEXT NOT NULL DEFAULT '',
			test_url TEXT NOT NULL DEFAULT '',
			egress_country TEXT NOT NULL DEFAULT '',
			egress_country_mode TEXT NOT NULL DEFAULT 'include',
			egress_countries_json TEXT NOT NULL DEFAULT '[]',
			node_source_mode TEXT NOT NULL DEFAULT 'all',
			source_ids_json TEXT NOT NULL DEFAULT '[]',
			protocols_json TEXT NOT NULL DEFAULT '[]',
			name_include_regex TEXT NOT NULL DEFAULT '',
			name_exclude_regex TEXT NOT NULL DEFAULT '',
			manual_only INTEGER NOT NULL DEFAULT 0,
			min_evaluation_interval_seconds INTEGER NOT NULL DEFAULT 0,
			candidate_limit INTEGER NOT NULL DEFAULT 0,
			relative_improvement_threshold REAL NOT NULL DEFAULT 0.2,
			absolute_latency_improvement_ms INTEGER NOT NULL DEFAULT 100,
			current_node_id TEXT NOT NULL DEFAULT '',
			current_exit_node_id TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL DEFAULT 'pending',
			last_error TEXT NOT NULL DEFAULT '',
			current_path_latency_ms INTEGER NOT NULL DEFAULT 0,
			current_path_failed_evaluations INTEGER NOT NULL DEFAULT 0,
			current_path_missed_success_cycles INTEGER NOT NULL DEFAULT 0,
			switch_reason TEXT NOT NULL DEFAULT '',
			last_evaluation_details_json TEXT NOT NULL DEFAULT '{}',
			config_version INTEGER NOT NULL DEFAULT 1,
			auto_evaluation_enabled INTEGER NOT NULL DEFAULT 1,
			auto_evaluation_interval_seconds INTEGER NOT NULL DEFAULT 0,
			node_sticky_enabled INTEGER NOT NULL DEFAULT 0,
			last_evaluation_started_at INTEGER NOT NULL DEFAULT 0,
			last_evaluated_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS maintenance_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			subscription_refresh_seconds INTEGER NOT NULL,
			node_observation_seconds INTEGER NOT NULL,
			profile_evaluation_seconds INTEGER NOT NULL,
			chain_evaluation_seconds INTEGER NOT NULL,
			geoip_update_time TEXT NOT NULL,
			egress_ip_probe_url TEXT NOT NULL,
			subscription_concurrency INTEGER NOT NULL,
			node_observation_concurrency INTEGER NOT NULL,
			profile_evaluation_concurrency INTEGER NOT NULL,
			geoip_concurrency INTEGER NOT NULL
		)`,
		`INSERT OR IGNORE INTO maintenance_settings (
			id,
			subscription_refresh_seconds,
			node_observation_seconds,
			profile_evaluation_seconds,
			chain_evaluation_seconds,
			geoip_update_time,
			egress_ip_probe_url,
			subscription_concurrency,
			node_observation_concurrency,
			profile_evaluation_concurrency,
			geoip_concurrency
		) VALUES (1, 21600, 1800, 300, 900, '07:00', 'https://cloudflare.com/cdn-cgi/trace', 1, 8, 2, 1)`,
		`CREATE TABLE IF NOT EXISTS maintenance_runs (
			id TEXT PRIMARY KEY,
			run_type TEXT NOT NULL,
			trigger_source TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			target_label TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL,
			result TEXT NOT NULL DEFAULT '',
			reason_code TEXT NOT NULL DEFAULT '',
			total_count INTEGER NOT NULL DEFAULT 0,
			finished_count INTEGER NOT NULL DEFAULT 0,
			detail_json TEXT NOT NULL DEFAULT '{}',
			last_error TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			started_at INTEGER NOT NULL DEFAULT 0,
			finished_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_maintenance_runs_created ON maintenance_runs (created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_maintenance_runs_state_created ON maintenance_runs (state, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_maintenance_runs_type_target_created ON maintenance_runs (run_type, target_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_maintenance_runs_type_state_created ON maintenance_runs (run_type, state, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS geoip_status (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			file_path TEXT NOT NULL DEFAULT '',
			loaded_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			sha256 TEXT NOT NULL DEFAULT ''
		)`,
		`INSERT OR IGNORE INTO geoip_status (id, file_path) VALUES (1, '')`,
		`CREATE TABLE IF NOT EXISTS evaluation_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			global_concurrency INTEGER NOT NULL,
			default_min_evaluation_interval_seconds INTEGER NOT NULL,
			single_candidate_limit INTEGER NOT NULL,
			chain_candidate_limit INTEGER NOT NULL,
			connect_timeout_seconds INTEGER NOT NULL DEFAULT 10,
			probe_timeout_seconds INTEGER NOT NULL DEFAULT 10
		)`,
		`INSERT OR IGNORE INTO evaluation_settings (
			id,
			global_concurrency,
			default_min_evaluation_interval_seconds,
			single_candidate_limit,
			chain_candidate_limit
		) VALUES (1, 32, 300, 0, 100)`,
		`CREATE TABLE IF NOT EXISTS proxy_credentials (
			id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL,
			remark TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			password_hash TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			last_used_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
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
			state TEXT NOT NULL DEFAULT 'completed',
			success INTEGER,
			failure_stage TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			ingress_bytes INTEGER NOT NULL DEFAULT 0,
			egress_bytes INTEGER NOT NULL DEFAULT 0,
			http_status INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS kv_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	if err := ensureColumn(db, "access_profiles", "min_evaluation_interval_seconds", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "candidate_limit", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "exit_node_ids_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "chain_evaluation_mode", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "egress_country_mode", "TEXT NOT NULL DEFAULT 'include'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "egress_countries_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "current_exit_node_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "protocols_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "relative_improvement_threshold", "REAL NOT NULL DEFAULT 0.2"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "absolute_latency_improvement_ms", "INTEGER NOT NULL DEFAULT 100"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "node_source_mode", "TEXT NOT NULL DEFAULT 'all'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "last_evaluation_started_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "last_evaluated_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "last_error", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "current_path_latency_ms", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "current_path_failed_evaluations", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "current_path_missed_success_cycles", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "switch_reason", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "last_evaluation_details_json", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "config_version", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "auto_evaluation_enabled", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "auto_evaluation_interval_seconds", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "node_sticky_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := dropColumnIfExists(db, "access_profiles", "maximum_hold_seconds"); err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM kv_settings WHERE key = 'switching_tolerance.maximum_hold_seconds'`); err != nil {
		return err
	}
	if err := ensureColumn(db, "nodes", "outbound_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "subscriptions", "skipped_summary_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "subscriptions", "auto_refresh_enabled", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := ensureColumn(db, "subscriptions", "auto_refresh_interval_seconds", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumn(db, "proxy_credentials", "password", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "proxy_credentials", "remark", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "access_profiles", "profile_identifier", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "evaluation_settings", "connect_timeout_seconds", "INTEGER NOT NULL DEFAULT 10"); err != nil {
		return err
	}
	if err := ensureColumn(db, "evaluation_settings", "probe_timeout_seconds", "INTEGER NOT NULL DEFAULT 10"); err != nil {
		return err
	}
	if err := ensureColumn(db, "request_logs", "proxy_credential_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "request_logs", "access_profile_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "request_logs", "access_profile_identifier", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "request_logs", "proxy_path_json", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumn(db, "request_logs", "state", "TEXT NOT NULL DEFAULT 'completed'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "request_logs", "failure_stage", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return ensureRequestLogsSchema(db)
}

func backfillNodeOutboundJSON(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT id, name, type, server, server_port, username, password, outbound_json FROM nodes ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	type row struct {
		id           string
		node         applicationnodes.OutboundNode
		outboundJSON string
	}
	var nodes []row
	for rows.Next() {
		var item row
		if err := rows.Scan(
			&item.id,
			&item.node.Name,
			&item.node.Type,
			&item.node.Server,
			&item.node.ServerPort,
			&item.node.Username,
			&item.node.Password,
			&item.outboundJSON,
		); err != nil {
			_ = rows.Close()
			return err
		}
		item.node.OutboundJSON = item.outboundJSON
		nodes = append(nodes, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range nodes {
		outboundJSON, err := applicationnodes.NormalizeOutboundJSON(item.node)
		if err != nil {
			return err
		}
		_, err = db.ExecContext(
			ctx,
			`UPDATE nodes SET outbound_json = ?, fingerprint = ? WHERE id = ?`,
			outboundJSON,
			applicationnodes.OutboundFingerprint(outboundJSON),
			item.id,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func ensureColumn(db schemaExecutor, table, column, definition string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if name == column {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

func dropColumnIfExists(db schemaExecutor, table, column string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if name == column {
			found = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !found {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` DROP COLUMN ` + column)
	return err
}

func ensureRequestLogsSchema(db schemaExecutor) error {
	rows, err := db.Query(`PRAGMA table_info(request_logs)`)
	if err != nil {
		return err
	}
	successFound := false
	successNotNull := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		if name == "success" {
			successFound = true
			successNotNull = notNull == 1
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if !successFound || !successNotNull {
		return closeInterruptedRequestLogs(db)
	}

	stmts := []string{
		`DROP TABLE IF EXISTS request_logs_next`,
		`CREATE TABLE request_logs_next (
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
			state TEXT NOT NULL DEFAULT 'completed',
			success INTEGER,
			failure_stage TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			ingress_bytes INTEGER NOT NULL DEFAULT 0,
			egress_bytes INTEGER NOT NULL DEFAULT 0,
			http_status INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO request_logs_next (
			id, ts, proxy_credential_id, proxy_credential, access_profile_id, access_profile, access_profile_identifier,
			target_host, proxy_path, proxy_path_json, state, success, failure_stage, error, duration_ms, ingress_bytes, egress_bytes, http_status
		 )
		 SELECT
			id, ts, proxy_credential_id, proxy_credential, access_profile_id, access_profile, access_profile_identifier,
			target_host, proxy_path, proxy_path_json,
			CASE WHEN state = '' THEN 'completed' ELSE state END,
			success, failure_stage, error, duration_ms, ingress_bytes, egress_bytes, http_status
		   FROM request_logs`,
		`DROP TABLE request_logs`,
		`ALTER TABLE request_logs_next RENAME TO request_logs`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return closeInterruptedRequestLogs(db)
}

func closeInterruptedRequestLogs(db interface {
	Exec(query string, args ...any) (sql.Result, error)
}) error {
	_, err := db.Exec(
		`UPDATE request_logs
		    SET state = 'completed',
		        success = 0,
		        error = CASE WHEN error = '' THEN 'gateway restarted before request completed' ELSE error END,
		        duration_ms = CASE WHEN duration_ms <= 0 THEN 1 ELSE duration_ms END
		  WHERE state = 'running'`,
	)
	return err
}
