package postgres

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"

	databaseinfra "proxygateway/internal/infrastructure/database"
)

const initialSchemaMigrationVersion int64 = 202606280001

func Migrate(ctx context.Context, db *sql.DB) error {
	ConfigureConnection(db)
	if err := databaseinfra.Migrate(ctx, db, databaseinfra.MigrationSet{
		Dialect: databaseinfra.DialectPostgres,
		GoMigrations: []*goose.Migration{
			goose.NewGoMigration(
				initialSchemaMigrationVersion,
				&goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
					return createBaselineSchema(ctx, tx)
				}},
				nil,
			),
		},
	}); err != nil {
		return err
	}
	return closeInterruptedRequestLogs(ctx, db)
}

func createBaselineSchema(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE admin_credentials (
			id integer PRIMARY KEY CHECK (id = 1),
			password_hash text NOT NULL,
			created_at bigint NOT NULL
		)`,
		`CREATE TABLE admin_sessions (
			token_hash text PRIMARY KEY,
			created_at bigint NOT NULL
		)`,
		`CREATE TABLE nodes (
			sequence bigserial UNIQUE,
			id text PRIMARY KEY,
			fingerprint text NOT NULL UNIQUE,
			name text NOT NULL,
			type text NOT NULL,
			server text NOT NULL DEFAULT '',
			server_port integer NOT NULL DEFAULT 0,
			username text NOT NULL DEFAULT '',
			password text NOT NULL DEFAULT '',
			raw_json text NOT NULL DEFAULT '',
			outbound_json text NOT NULL DEFAULT '',
			source_id text NOT NULL DEFAULT '',
			enabled boolean NOT NULL DEFAULT true,
			created_at bigint NOT NULL
		)`,
		`CREATE INDEX idx_nodes_created_id ON nodes (created_at, id)`,
		`CREATE INDEX idx_nodes_enabled_created ON nodes (enabled, created_at, id)`,
		`CREATE INDEX idx_nodes_lower_name ON nodes (lower(name))`,
		`CREATE INDEX idx_nodes_type ON nodes (type)`,
		`CREATE TABLE node_sources (
			node_id text NOT NULL,
			source_id text NOT NULL,
			source_name text NOT NULL,
			source_type text NOT NULL,
			display_name text NOT NULL,
			created_at bigint NOT NULL,
			PRIMARY KEY (node_id, source_id)
		)`,
		`CREATE INDEX idx_node_sources_source ON node_sources (source_type, source_id)`,
		`CREATE INDEX idx_node_sources_node_created ON node_sources (node_id, created_at, source_id)`,
		`CREATE TABLE retained_profile_nodes (
			profile_id text NOT NULL,
			node_id text NOT NULL,
			created_at bigint NOT NULL,
			PRIMARY KEY (profile_id, node_id)
		)`,
		`CREATE INDEX idx_retained_profile_nodes_node ON retained_profile_nodes (node_id)`,
		`CREATE TABLE node_observations (
			node_id text PRIMARY KEY,
			usable boolean NOT NULL DEFAULT false,
			egress_ip text NOT NULL DEFAULT '',
			egress_country text NOT NULL DEFAULT '',
			latency_ms bigint NOT NULL DEFAULT 0,
			last_error text NOT NULL DEFAULT '',
			last_success_at bigint NOT NULL DEFAULT 0,
			last_failure_at bigint NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX idx_node_observations_usable_country ON node_observations (usable, egress_country)`,
		`CREATE TABLE subscriptions (
			id text PRIMARY KEY,
			name text NOT NULL,
			source_type text NOT NULL,
			url text NOT NULL DEFAULT '',
			content text NOT NULL DEFAULT '',
			imported_nodes integer NOT NULL DEFAULT 0,
			skipped_entries integer NOT NULL DEFAULT 0,
			skipped_summary_json text NOT NULL DEFAULT '[]',
			last_error text NOT NULL DEFAULT '',
			auto_refresh_enabled boolean NOT NULL DEFAULT true,
			auto_refresh_interval_seconds bigint NOT NULL DEFAULT 0,
			created_at bigint NOT NULL,
			updated_at bigint NOT NULL
		)`,
		`CREATE INDEX idx_subscriptions_auto_refresh ON subscriptions (auto_refresh_enabled, updated_at)`,
		`CREATE INDEX idx_subscriptions_created_id ON subscriptions (created_at, id)`,
		`CREATE TABLE access_profiles (
			sequence bigserial UNIQUE,
			id text PRIMARY KEY,
			profile_identifier text NOT NULL DEFAULT '',
			name text NOT NULL,
			type text NOT NULL,
			fixed_node_id text NOT NULL DEFAULT '',
			exit_node_ids_json text NOT NULL DEFAULT '[]',
			chain_evaluation_mode text NOT NULL DEFAULT '',
			test_url text NOT NULL DEFAULT '',
			egress_country text NOT NULL DEFAULT '',
			egress_country_mode text NOT NULL DEFAULT 'include',
			egress_countries_json text NOT NULL DEFAULT '[]',
			node_source_mode text NOT NULL DEFAULT 'all',
			source_ids_json text NOT NULL DEFAULT '[]',
			protocols_json text NOT NULL DEFAULT '[]',
			name_include_regex text NOT NULL DEFAULT '',
			name_exclude_regex text NOT NULL DEFAULT '',
			manual_only boolean NOT NULL DEFAULT false,
			min_evaluation_interval_seconds bigint NOT NULL DEFAULT 0,
			candidate_limit integer NOT NULL DEFAULT 0,
			relative_improvement_threshold double precision NOT NULL DEFAULT 0.2,
			absolute_latency_improvement_ms bigint NOT NULL DEFAULT 100,
			current_node_id text NOT NULL DEFAULT '',
			current_exit_node_id text NOT NULL DEFAULT '',
			state text NOT NULL DEFAULT 'pending',
			last_error text NOT NULL DEFAULT '',
			current_path_latency_ms bigint NOT NULL DEFAULT 0,
			current_path_failed_evaluations integer NOT NULL DEFAULT 0,
			current_path_missed_success_cycles integer NOT NULL DEFAULT 0,
			switch_reason text NOT NULL DEFAULT '',
			last_evaluation_details_json text NOT NULL DEFAULT '{}',
			config_version bigint NOT NULL DEFAULT 1,
			auto_evaluation_enabled boolean NOT NULL DEFAULT true,
			auto_evaluation_interval_seconds bigint NOT NULL DEFAULT 0,
			node_sticky_enabled boolean NOT NULL DEFAULT false,
			last_evaluation_started_at bigint NOT NULL DEFAULT 0,
			last_evaluated_at bigint NOT NULL DEFAULT 0,
			created_at bigint NOT NULL
		)`,
		`CREATE INDEX idx_access_profiles_created_id ON access_profiles (created_at, id)`,
		`CREATE INDEX idx_access_profiles_identifier ON access_profiles (profile_identifier)`,
		`CREATE INDEX idx_access_profiles_state ON access_profiles (state)`,
		`CREATE INDEX idx_access_profiles_auto_eval ON access_profiles (auto_evaluation_enabled, last_evaluated_at)`,
		`CREATE INDEX idx_access_profiles_current_nodes ON access_profiles (current_node_id, current_exit_node_id)`,
		`CREATE TABLE maintenance_settings (
			id integer PRIMARY KEY CHECK (id = 1),
			subscription_refresh_seconds bigint NOT NULL,
			node_observation_seconds bigint NOT NULL,
			profile_evaluation_seconds bigint NOT NULL,
			chain_evaluation_seconds bigint NOT NULL,
			geoip_update_time text NOT NULL,
			egress_ip_probe_url text NOT NULL,
			subscription_concurrency integer NOT NULL,
			node_observation_concurrency integer NOT NULL,
			profile_evaluation_concurrency integer NOT NULL,
			geoip_concurrency integer NOT NULL
		)`,
		`INSERT INTO maintenance_settings (
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
		`CREATE TABLE maintenance_runs (
			sequence bigserial UNIQUE,
			id text PRIMARY KEY,
			run_type text NOT NULL,
			trigger_source text NOT NULL DEFAULT '',
			target_id text NOT NULL DEFAULT '',
			target_label text NOT NULL DEFAULT '',
			state text NOT NULL,
			result text NOT NULL DEFAULT '',
			reason_code text NOT NULL DEFAULT '',
			total_count integer NOT NULL DEFAULT 0,
			finished_count integer NOT NULL DEFAULT 0,
			detail_json text NOT NULL DEFAULT '{}',
			last_error text NOT NULL DEFAULT '',
			created_at bigint NOT NULL,
			started_at bigint NOT NULL DEFAULT 0,
			finished_at bigint NOT NULL DEFAULT 0,
			updated_at bigint NOT NULL
		)`,
		`CREATE INDEX idx_maintenance_runs_created ON maintenance_runs (created_at DESC, sequence DESC)`,
		`CREATE INDEX idx_maintenance_runs_state_created ON maintenance_runs (state, created_at DESC, sequence DESC)`,
		`CREATE INDEX idx_maintenance_runs_type_target_created ON maintenance_runs (run_type, target_id, created_at DESC, sequence DESC)`,
		`CREATE INDEX idx_maintenance_runs_type_state_created ON maintenance_runs (run_type, state, created_at ASC, sequence ASC)`,
		`CREATE TABLE geoip_status (
			id integer PRIMARY KEY CHECK (id = 1),
			file_path text NOT NULL DEFAULT '',
			loaded_at bigint NOT NULL DEFAULT 0,
			updated_at bigint NOT NULL DEFAULT 0,
			last_error text NOT NULL DEFAULT '',
			sha256 text NOT NULL DEFAULT ''
		)`,
		`INSERT INTO geoip_status (id, file_path) VALUES (1, '')`,
		`CREATE TABLE evaluation_settings (
			id integer PRIMARY KEY CHECK (id = 1),
			global_concurrency integer NOT NULL,
			default_min_evaluation_interval_seconds bigint NOT NULL,
			single_candidate_limit integer NOT NULL,
			chain_candidate_limit integer NOT NULL,
			connect_timeout_seconds bigint NOT NULL DEFAULT 10,
			probe_timeout_seconds bigint NOT NULL DEFAULT 10
		)`,
		`INSERT INTO evaluation_settings (
			id,
			global_concurrency,
			default_min_evaluation_interval_seconds,
			single_candidate_limit,
			chain_candidate_limit
		) VALUES (1, 32, 300, 0, 100)`,
		`CREATE TABLE proxy_credentials (
			sequence bigserial UNIQUE,
			id text PRIMARY KEY,
			profile_id text NOT NULL,
			remark text NOT NULL DEFAULT '',
			password text NOT NULL DEFAULT '',
			password_hash text NOT NULL,
			enabled boolean NOT NULL DEFAULT true,
			created_at bigint NOT NULL,
			last_used_at bigint NOT NULL DEFAULT 0,
			CONSTRAINT fk_proxy_credentials_profile
				FOREIGN KEY (profile_id) REFERENCES access_profiles(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX idx_proxy_credentials_profile ON proxy_credentials (profile_id, created_at, id)`,
		`CREATE INDEX idx_proxy_credentials_lookup ON proxy_credentials (profile_id, password, enabled)`,
		`CREATE TABLE request_logs (
			sequence bigserial UNIQUE,
			id text PRIMARY KEY,
			ts bigint NOT NULL,
			proxy_credential_id text NOT NULL DEFAULT '',
			proxy_credential text NOT NULL,
			access_profile_id text NOT NULL DEFAULT '',
			access_profile text NOT NULL,
			access_profile_identifier text NOT NULL DEFAULT '',
			target_host text NOT NULL,
			proxy_path text NOT NULL,
			proxy_path_json text NOT NULL DEFAULT '',
			state text NOT NULL DEFAULT 'completed',
			success boolean,
			failure_stage text NOT NULL DEFAULT '',
			error text NOT NULL DEFAULT '',
			duration_ms bigint NOT NULL DEFAULT 0,
			ingress_bytes bigint NOT NULL DEFAULT 0,
			egress_bytes bigint NOT NULL DEFAULT 0,
			http_status integer NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX idx_request_logs_ts ON request_logs (ts DESC, sequence DESC)`,
		`CREATE INDEX idx_request_logs_state_success_ts ON request_logs (state, success, ts DESC, sequence DESC)`,
		`CREATE INDEX idx_request_logs_profile_ts ON request_logs (access_profile_id, access_profile, ts DESC)`,
		`CREATE INDEX idx_request_logs_credential_ts ON request_logs (proxy_credential_id, proxy_credential, ts DESC)`,
		`CREATE INDEX idx_request_logs_target_host ON request_logs (target_host)`,
		`CREATE INDEX idx_request_logs_proxy_path_json ON request_logs (proxy_path_json)`,
		`CREATE TABLE kv_settings (
			key text PRIMARY KEY,
			value text NOT NULL DEFAULT ''
		)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func closeInterruptedRequestLogs(ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}) error {
	_, err := db.ExecContext(
		ctx,
		`UPDATE request_logs
		    SET state = 'completed',
		        success = false,
		        error = CASE WHEN error = '' THEN 'gateway restarted before request completed' ELSE error END,
		        duration_ms = CASE WHEN duration_ms <= 0 THEN 1 ELSE duration_ms END
		  WHERE state = 'running'`,
	)
	return err
}
