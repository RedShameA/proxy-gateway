package sqlite

import (
	"context"
	"database/sql"

	appsettings "proxygateway/internal/application/settings"
)

type SystemSettingsRepository struct {
	db *sql.DB
}

func NewSystemSettingsRepository(db *sql.DB) SystemSettingsRepository {
	return SystemSettingsRepository{db: db}
}

func (r SystemSettingsRepository) LoadEvaluation(ctx context.Context) (appsettings.EvaluationSettings, error) {
	var settings appsettings.EvaluationSettings
	err := r.db.QueryRowContext(
		ctx,
		`SELECT global_concurrency,
		        default_min_evaluation_interval_seconds,
		        single_candidate_limit,
		        chain_candidate_limit,
		        connect_timeout_seconds,
		        probe_timeout_seconds
		   FROM evaluation_settings
		  WHERE id = 1`,
	).Scan(
		&settings.GlobalConcurrency,
		&settings.DefaultMinEvaluationIntervalSeconds,
		&settings.SingleCandidateLimit,
		&settings.ChainCandidateLimit,
		&settings.ConnectTimeoutSeconds,
		&settings.ProbeTimeoutSeconds,
	)
	return settings, err
}

func (r SystemSettingsRepository) SaveEvaluation(ctx context.Context, settings appsettings.EvaluationSettings) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE evaluation_settings
		 SET global_concurrency = ?,
		     default_min_evaluation_interval_seconds = ?,
		     single_candidate_limit = ?,
		     chain_candidate_limit = ?,
		     connect_timeout_seconds = ?,
		     probe_timeout_seconds = ?
		 WHERE id = 1`,
		settings.GlobalConcurrency,
		settings.DefaultMinEvaluationIntervalSeconds,
		settings.SingleCandidateLimit,
		settings.ChainCandidateLimit,
		settings.ConnectTimeoutSeconds,
		settings.ProbeTimeoutSeconds,
	)
	return err
}

func (r SystemSettingsRepository) LoadMaintenance(ctx context.Context) (appsettings.MaintenanceSettings, error) {
	var settings appsettings.MaintenanceSettings
	err := r.db.QueryRowContext(
		ctx,
		`SELECT subscription_refresh_seconds,
		        node_observation_seconds,
		        profile_evaluation_seconds,
		        chain_evaluation_seconds,
		        geoip_update_time,
		        egress_ip_probe_url,
		        subscription_concurrency,
		        node_observation_concurrency,
		        profile_evaluation_concurrency,
		        geoip_concurrency
		   FROM maintenance_settings WHERE id = 1`,
	).Scan(
		&settings.SubscriptionRefreshSeconds,
		&settings.NodeObservationSeconds,
		&settings.ProfileEvaluationSeconds,
		&settings.ChainEvaluationSeconds,
		&settings.GeoIPUpdateTime,
		&settings.EgressIPProbeURL,
		&settings.SubscriptionConcurrency,
		&settings.NodeObservationConcurrency,
		&settings.ProfileEvaluationConcurrency,
		&settings.GeoIPConcurrency,
	)
	return settings, err
}

func (r SystemSettingsRepository) SaveMaintenance(ctx context.Context, settings appsettings.MaintenanceSettings) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE maintenance_settings
		    SET subscription_refresh_seconds = ?,
		        node_observation_seconds = ?,
		        profile_evaluation_seconds = ?,
		        chain_evaluation_seconds = ?,
		        geoip_update_time = ?,
		        egress_ip_probe_url = ?,
		        subscription_concurrency = ?,
		        node_observation_concurrency = ?,
		        profile_evaluation_concurrency = ?,
		        geoip_concurrency = ?
		  WHERE id = 1`,
		settings.SubscriptionRefreshSeconds,
		settings.NodeObservationSeconds,
		settings.ProfileEvaluationSeconds,
		settings.ChainEvaluationSeconds,
		settings.GeoIPUpdateTime,
		settings.EgressIPProbeURL,
		settings.SubscriptionConcurrency,
		settings.NodeObservationConcurrency,
		settings.ProfileEvaluationConcurrency,
		settings.GeoIPConcurrency,
	)
	return err
}
