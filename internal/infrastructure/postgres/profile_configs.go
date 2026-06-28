package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	appprofiles "proxygateway/internal/application/profiles"
)

type ProfileConfigRepository struct {
	db *sql.DB
}

type ProfileConfigRepositoryTx struct {
	tx *sql.Tx
}

func NewProfileConfigRepository(db *sql.DB) ProfileConfigRepository {
	return ProfileConfigRepository{db: db}
}

func NewProfileConfigRepositoryTx(tx *sql.Tx) ProfileConfigRepositoryTx {
	return ProfileConfigRepositoryTx{tx: tx}
}

func (r ProfileConfigRepository) ListConfigIDs(ctx context.Context, filter appprofiles.ListConfigFilter) (appprofiles.ListConfigResult, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM access_profiles`).Scan(&total); err != nil {
		return appprofiles.ListConfigResult{}, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM access_profiles ORDER BY created_at, id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return appprofiles.ListConfigResult{}, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return appprofiles.ListConfigResult{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return appprofiles.ListConfigResult{}, err
	}
	return appprofiles.ListConfigResult{IDs: ids, Total: total}, nil
}

func (r ProfileConfigRepository) LoadConfig(ctx context.Context, profileID string) (appprofiles.ConfigRecord, bool, error) {
	record, err := scanProfileConfig(r.db.QueryRowContext(ctx, profileConfigSelectSQL+` WHERE id = $1`, profileID))
	if err == sql.ErrNoRows {
		return appprofiles.ConfigRecord{}, false, nil
	}
	if err != nil {
		return appprofiles.ConfigRecord{}, false, err
	}
	return record, true, nil
}

func (r ProfileConfigRepository) CreateConfig(ctx context.Context, record appprofiles.ConfigRecord, createdAt int64) error {
	_, err := r.db.ExecContext(ctx, profileConfigInsertSQL,
		record.ID,
		record.ProfileIdentifier,
		record.Name,
		record.Type,
		record.FixedNodeID,
		stringSliceJSON(record.ExitNodeIDs),
		record.ChainEvaluationMode,
		record.TestURL,
		record.EgressCountry,
		record.EgressCountryMode,
		stringSliceJSON(record.EgressCountries),
		record.NodeSourceMode,
		stringSliceJSON(record.SourceIDs),
		stringSliceJSON(record.Protocols),
		record.NameIncludeRegex,
		record.NameExcludeRegex,
		record.ManualOnly,
		record.MinEvaluationIntervalSeconds,
		record.CandidateLimit,
		record.RelativeImprovementThreshold,
		record.AbsoluteLatencyImprovementMS,
		record.CurrentNodeID,
		record.CurrentExitNodeID,
		record.State,
		record.AutoEvaluationEnabled,
		record.AutoEvaluationInterval,
		record.NodeStickyEnabled,
		record.ConfigVersion,
		createdAt,
	)
	return err
}

func (r ProfileConfigRepository) UpdateConfig(ctx context.Context, record appprofiles.ConfigRecord, options appprofiles.ConfigUpdateOptions) error {
	return updateProfileConfig(ctx, r.db, record, options)
}

func (r ProfileConfigRepositoryTx) UpdateConfig(ctx context.Context, record appprofiles.ConfigRecord, options appprofiles.ConfigUpdateOptions) error {
	return updateProfileConfig(ctx, r.tx, record, options)
}

func (r ProfileConfigRepository) ProfileIdentifierExists(ctx context.Context, identifier, excludeProfileID string) (bool, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM access_profiles WHERE profile_identifier = $1 AND id != $2`, identifier, excludeProfileID).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r ProfileConfigRepository) Exists(ctx context.Context, profileID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM access_profiles WHERE id = $1`, profileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

type profileConfigExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func updateProfileConfig(ctx context.Context, exec profileConfigExecutor, record appprofiles.ConfigRecord, options appprofiles.ConfigUpdateOptions) error {
	_, err := exec.ExecContext(ctx, profileConfigUpdateSQL,
		record.Name,
		record.ProfileIdentifier,
		record.Type,
		record.FixedNodeID,
		stringSliceJSON(record.ExitNodeIDs),
		record.ChainEvaluationMode,
		record.TestURL,
		record.EgressCountry,
		record.EgressCountryMode,
		stringSliceJSON(record.EgressCountries),
		record.NodeSourceMode,
		stringSliceJSON(record.SourceIDs),
		stringSliceJSON(record.Protocols),
		record.NameIncludeRegex,
		record.NameExcludeRegex,
		record.ManualOnly,
		record.MinEvaluationIntervalSeconds,
		record.CandidateLimit,
		record.RelativeImprovementThreshold,
		record.AbsoluteLatencyImprovementMS,
		record.CurrentNodeID,
		record.CurrentExitNodeID,
		record.State,
		record.AutoEvaluationEnabled,
		record.AutoEvaluationInterval,
		record.NodeStickyEnabled,
		options.EvaluationChanged,
		options.ResetCurrentPath,
		options.ResetCurrentPath,
		options.ResetCurrentPath,
		record.SwitchReason,
		options.ResetCurrentPath,
		record.LastEvaluationDetailsJSON,
		options.ResetCurrentPath,
		options.ResetCurrentPath,
		record.ConfigVersion,
		record.ID,
	)
	return err
}

const profileConfigSelectSQL = `SELECT id, name, profile_identifier, type, fixed_node_id, exit_node_ids_json, chain_evaluation_mode, test_url,
        egress_country, egress_country_mode, egress_countries_json, node_source_mode, source_ids_json, protocols_json,
        name_include_regex, name_exclude_regex, manual_only, min_evaluation_interval_seconds, candidate_limit,
        relative_improvement_threshold, absolute_latency_improvement_ms,
        current_node_id, current_exit_node_id, state, last_evaluated_at, last_error, current_path_latency_ms,
        switch_reason, last_evaluation_details_json, auto_evaluation_enabled, auto_evaluation_interval_seconds, node_sticky_enabled, config_version
   FROM access_profiles`

const profileConfigInsertSQL = `INSERT INTO access_profiles (
	id, profile_identifier, name, type, fixed_node_id, exit_node_ids_json, chain_evaluation_mode, test_url,
	egress_country, egress_country_mode, egress_countries_json, node_source_mode, source_ids_json, protocols_json,
	name_include_regex, name_exclude_regex, manual_only, min_evaluation_interval_seconds, candidate_limit,
	relative_improvement_threshold, absolute_latency_improvement_ms,
	current_node_id, current_exit_node_id, state, auto_evaluation_enabled, auto_evaluation_interval_seconds, node_sticky_enabled, config_version, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)`

const profileConfigUpdateSQL = `UPDATE access_profiles
    SET name = $1,
        profile_identifier = $2,
        type = $3,
        fixed_node_id = $4,
        exit_node_ids_json = $5,
        chain_evaluation_mode = $6,
        test_url = $7,
        egress_country = $8,
        egress_country_mode = $9,
        egress_countries_json = $10,
        node_source_mode = $11,
        source_ids_json = $12,
        protocols_json = $13,
        name_include_regex = $14,
        name_exclude_regex = $15,
        manual_only = $16,
        min_evaluation_interval_seconds = $17,
        candidate_limit = $18,
        relative_improvement_threshold = $19,
        absolute_latency_improvement_ms = $20,
        current_node_id = $21,
        current_exit_node_id = $22,
        state = $23,
        auto_evaluation_enabled = $24,
        auto_evaluation_interval_seconds = $25,
        node_sticky_enabled = $26,
        last_error = CASE WHEN $27 THEN '' ELSE last_error END,
        current_path_failed_evaluations = CASE WHEN $28 THEN 0 ELSE current_path_failed_evaluations END,
        current_path_missed_success_cycles = CASE WHEN $29 THEN 0 ELSE current_path_missed_success_cycles END,
        switch_reason = CASE WHEN $30 THEN $31 ELSE switch_reason END,
        last_evaluation_details_json = CASE WHEN $32 THEN $33 ELSE last_evaluation_details_json END,
        last_evaluated_at = CASE WHEN $34 THEN 0 ELSE last_evaluated_at END,
        last_evaluation_started_at = CASE WHEN $35 THEN 0 ELSE last_evaluation_started_at END,
        config_version = $36
  WHERE id = $37`

func scanProfileConfig(row profileConfigScanner) (appprofiles.ConfigRecord, error) {
	var record appprofiles.ConfigRecord
	var sourceIDsJSON, protocolsJSON, egressCountriesJSON, exitNodeIDsJSON string
	var minInterval, absoluteImprovement, autoEvalInterval int64
	err := row.Scan(
		&record.ID,
		&record.Name,
		&record.ProfileIdentifier,
		&record.Type,
		&record.FixedNodeID,
		&exitNodeIDsJSON,
		&record.ChainEvaluationMode,
		&record.TestURL,
		&record.EgressCountry,
		&record.EgressCountryMode,
		&egressCountriesJSON,
		&record.NodeSourceMode,
		&sourceIDsJSON,
		&protocolsJSON,
		&record.NameIncludeRegex,
		&record.NameExcludeRegex,
		&record.ManualOnly,
		&minInterval,
		&record.CandidateLimit,
		&record.RelativeImprovementThreshold,
		&absoluteImprovement,
		&record.CurrentNodeID,
		&record.CurrentExitNodeID,
		&record.State,
		&record.LastEvaluatedAt,
		&record.LastError,
		&record.CurrentPathLatencyMS,
		&record.SwitchReason,
		&record.LastEvaluationDetailsJSON,
		&record.AutoEvaluationEnabled,
		&autoEvalInterval,
		&record.NodeStickyEnabled,
		&record.ConfigVersion,
	)
	if err != nil {
		return appprofiles.ConfigRecord{}, err
	}
	record.MinEvaluationIntervalSeconds = int(minInterval)
	record.AbsoluteLatencyImprovementMS = int(absoluteImprovement)
	record.AutoEvaluationInterval = int(autoEvalInterval)
	record.ExitNodeIDs = unmarshalStringSlice(exitNodeIDsJSON)
	record.EgressCountries = unmarshalStringSlice(egressCountriesJSON)
	record.SourceIDs = unmarshalStringSlice(sourceIDsJSON)
	record.Protocols = unmarshalStringSlice(protocolsJSON)
	return record, nil
}

type profileConfigScanner interface {
	Scan(dest ...any) error
}

func stringSliceJSON(values []string) string {
	if values == nil {
		values = []string{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func unmarshalStringSlice(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil || values == nil {
		return []string{}
	}
	return values
}

var _ appprofiles.ConfigRepository = ProfileConfigRepository{}
