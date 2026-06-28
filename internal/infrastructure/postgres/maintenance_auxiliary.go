package postgres

import (
	"context"
	"database/sql"

	appmaintenance "proxygateway/internal/application/maintenance"
)

type MaintenanceAuxiliaryRepository struct {
	db *sql.DB
}

func NewMaintenanceAuxiliaryRepository(db *sql.DB) MaintenanceAuxiliaryRepository {
	return MaintenanceAuxiliaryRepository{db: db}
}

func (r MaintenanceAuxiliaryRepository) ListNodeObservationScheduleTargets(ctx context.Context) ([]appmaintenance.NodeObservationScheduleTarget, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT n.id, n.name
		   FROM nodes n
		  WHERE n.enabled = true
		    AND NOT (
		      NOT EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id)
		      AND EXISTS (SELECT 1 FROM retained_profile_nodes rp WHERE rp.node_id = n.id)
		    )
		  ORDER BY n.created_at, n.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []appmaintenance.NodeObservationScheduleTarget
	for rows.Next() {
		var target appmaintenance.NodeObservationScheduleTarget
		if err := rows.Scan(&target.ID, &target.Name); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func (r MaintenanceAuxiliaryRepository) ListSubscriptionNodeObservationTargets(ctx context.Context, subscriptionID string) ([]appmaintenance.NodeObservationScheduleTarget, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT n.id, n.name
		   FROM nodes n
		   JOIN node_sources s ON s.node_id = n.id
		  WHERE s.source_id = $1 AND s.source_type = 'subscription' AND n.enabled = true
		  ORDER BY n.created_at, n.id`,
		subscriptionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []appmaintenance.NodeObservationScheduleTarget
	for rows.Next() {
		var target appmaintenance.NodeObservationScheduleTarget
		if err := rows.Scan(&target.ID, &target.Name); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func (r MaintenanceAuxiliaryRepository) ListProfileEvaluationScheduleTargets(ctx context.Context) ([]appmaintenance.ProfileEvaluationScheduleTarget, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, type, last_evaluated_at, auto_evaluation_enabled, auto_evaluation_interval_seconds, config_version
		   FROM access_profiles
		  WHERE type IN ('fastest', 'chain')
		  ORDER BY created_at, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []appmaintenance.ProfileEvaluationScheduleTarget
	for rows.Next() {
		var target appmaintenance.ProfileEvaluationScheduleTarget
		if err := rows.Scan(
			&target.ID,
			&target.Name,
			&target.ProfileType,
			&target.LastEvaluatedAt,
			&target.AutoEvaluationEnabled,
			&target.AutoEvaluationIntervalSeconds,
			&target.ConfigVersion,
		); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func (r MaintenanceAuxiliaryRepository) ListProfilesWaitingForObservation(ctx context.Context) ([]appmaintenance.WaitingObservationProfile, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, name, config_version
		   FROM access_profiles
		  WHERE state = 'waiting_observation'
		    AND auto_evaluation_enabled = true
		    AND type IN ('fastest', 'chain')
		  ORDER BY created_at, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var profiles []appmaintenance.WaitingObservationProfile
	for rows.Next() {
		var profile appmaintenance.WaitingObservationProfile
		if err := rows.Scan(&profile.ID, &profile.Name, &profile.ConfigVersion); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (r MaintenanceAuxiliaryRepository) ListSubscriptionRefreshScheduleTargets(ctx context.Context) ([]appmaintenance.SubscriptionRefreshScheduleTarget, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, updated_at, auto_refresh_enabled, auto_refresh_interval_seconds
		   FROM subscriptions
		  WHERE source_type = 'remote'
		  ORDER BY created_at, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []appmaintenance.SubscriptionRefreshScheduleTarget
	for rows.Next() {
		var target appmaintenance.SubscriptionRefreshScheduleTarget
		if err := rows.Scan(
			&target.ID,
			&target.Name,
			&target.UpdatedAt,
			&target.AutoRefreshEnabled,
			&target.AutoRefreshIntervalSeconds,
		); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func (r MaintenanceAuxiliaryRepository) HasRecentRun(ctx context.Context, runType string, createdAfterMillis int64) (bool, error) {
	var recent int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM maintenance_runs WHERE run_type = $1 AND created_at > $2`,
		runType,
		createdAfterMillis,
	).Scan(&recent)
	if err != nil {
		return false, err
	}
	return recent > 0, nil
}

func (r MaintenanceAuxiliaryRepository) HasUnfinishedCurrentNodeObservedEvaluation(ctx context.Context, profileID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT 1
		   FROM maintenance_runs
		  WHERE run_type = 'profile_evaluation'
		    AND target_id = $1
		    AND trigger_source = 'current_node_observed'
		    AND state IN ('queued', 'running')
		  LIMIT 1`,
		profileID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (r MaintenanceAuxiliaryRepository) DeleteHistoryBefore(ctx context.Context, cutoffMillis int64, keepRunID string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM maintenance_runs WHERE created_at < $1 AND id != $2`, cutoffMillis, keepRunID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

var _ appmaintenance.AuxiliaryRepository = MaintenanceAuxiliaryRepository{}
