package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	appevaluations "proxygateway/internal/application/evaluations"
	domainprofile "proxygateway/internal/domain/profile"
)

type EvaluationRepository struct {
	db *sql.DB
}

func NewEvaluationRepository(db *sql.DB) EvaluationRepository {
	return EvaluationRepository{db: db}
}

func (r EvaluationRepository) ListTargets(ctx context.Context) ([]appevaluations.TargetRecord, error) {
	rows, err := r.db.QueryContext(
		ctx,
		evaluationTargetSelectSQL+`
		 WHERE type IN ($1, $2)
		 ORDER BY created_at, id`,
		domainprofile.TypeFastest,
		domainprofile.TypeChain,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []appevaluations.TargetRecord
	for rows.Next() {
		target, err := scanEvaluationTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func (r EvaluationRepository) LoadTarget(ctx context.Context, profileID string) (appevaluations.TargetRecord, bool, error) {
	target, err := scanEvaluationTarget(r.db.QueryRowContext(ctx, evaluationTargetSelectSQL+` WHERE id = $1`, profileID))
	if err == sql.ErrNoRows {
		return appevaluations.TargetRecord{}, false, nil
	}
	if err != nil {
		return appevaluations.TargetRecord{}, false, err
	}
	return target, true, nil
}

func (r EvaluationRepository) CurrentConfigVersion(ctx context.Context, profileID string) (int64, error) {
	var current int64
	err := r.db.QueryRowContext(ctx, `SELECT config_version FROM access_profiles WHERE id = $1`, profileID).Scan(&current)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return current, err
}

func (r EvaluationRepository) LastError(ctx context.Context, profileID string) (string, error) {
	var lastError string
	err := r.db.QueryRowContext(ctx, `SELECT last_error FROM access_profiles WHERE id = $1`, profileID).Scan(&lastError)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return lastError, err
}

func (r EvaluationRepository) CurrentPathCounters(ctx context.Context, profileID string) (appevaluations.PathCounters, error) {
	var counters appevaluations.PathCounters
	err := r.db.QueryRowContext(
		ctx,
		`SELECT current_path_failed_evaluations, current_path_missed_success_cycles FROM access_profiles WHERE id = $1`,
		profileID,
	).Scan(&counters.FailedEvaluations, &counters.MissedSuccessCycles)
	if err == sql.ErrNoRows {
		return appevaluations.PathCounters{}, nil
	}
	return counters, err
}

func (r EvaluationRepository) CurrentPathLatency(ctx context.Context, profileID string) (int64, error) {
	var latency int64
	err := r.db.QueryRowContext(ctx, `SELECT current_path_latency_ms FROM access_profiles WHERE id = $1`, profileID).Scan(&latency)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return latency, err
}

func (r EvaluationRepository) CurrentNodeID(ctx context.Context, profileID string) (string, error) {
	var nodeID string
	err := r.db.QueryRowContext(ctx, `SELECT current_node_id FROM access_profiles WHERE id = $1`, profileID).Scan(&nodeID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return nodeID, err
}

func (r EvaluationRepository) CurrentChainPath(ctx context.Context, profileID string) (appevaluations.ChainPath, error) {
	var path appevaluations.ChainPath
	err := r.db.QueryRowContext(ctx, `SELECT current_node_id, current_exit_node_id FROM access_profiles WHERE id = $1`, profileID).Scan(&path.FrontNodeID, &path.ExitNodeID)
	if err == sql.ErrNoRows {
		return appevaluations.ChainPath{}, nil
	}
	return path, err
}

func (r EvaluationRepository) ListCandidateNodeIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT n.id
		   FROM nodes n
		  WHERE n.enabled = true
		    AND NOT (
		      NOT EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id)
		      AND EXISTS (SELECT 1 FROM retained_profile_nodes r WHERE r.node_id = n.id)
		    )
		  ORDER BY n.created_at, n.sequence, n.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodeIDs []string
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err != nil {
			return nil, err
		}
		nodeIDs = append(nodeIDs, nodeID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodeIDs, nil
}

func (r EvaluationRepository) CandidateEgressCountry(ctx context.Context, nodeID string) (string, bool, error) {
	var country string
	err := r.db.QueryRowContext(ctx, `SELECT egress_country FROM node_observations WHERE node_id = $1 AND usable = true`, nodeID).Scan(&country)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return country, true, nil
}

func (r EvaluationRepository) ListCandidateSourceRefs(ctx context.Context, nodeID string) ([]appevaluations.SourceRef, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT source_type, source_id
		   FROM node_sources
		  WHERE node_id = $1
		  ORDER BY created_at, source_type, source_id`,
		nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []appevaluations.SourceRef
	for rows.Next() {
		var ref appevaluations.SourceRef
		if err := rows.Scan(&ref.SourceType, &ref.SourceID); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func (r EvaluationRepository) ProfileRetainsNode(ctx context.Context, profileID, nodeID string) (bool, error) {
	if strings.TrimSpace(profileID) == "" || strings.TrimSpace(nodeID) == "" {
		return false, nil
	}
	var exists int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT 1 FROM retained_profile_nodes WHERE profile_id = $1 AND node_id = $2 LIMIT 1`,
		profileID,
		nodeID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (r EvaluationRepository) UpdateProfileState(ctx context.Context, profileID string, configVersion int64, update appevaluations.StateUpdate) (bool, error) {
	return updateEvaluationProfileState(ctx, r.db, profileID, configVersion, update)
}

func (r EvaluationRepository) UpdateProfileStateAndReleaseRetained(ctx context.Context, profileID string, configVersion int64, keepNodeIDs []string, releaseRetained bool, update appevaluations.StateUpdate) (appevaluations.StateReleaseResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return appevaluations.StateReleaseResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	updated, err := updateEvaluationProfileState(ctx, tx, profileID, configVersion, update)
	if err != nil || !updated {
		return appevaluations.StateReleaseResult{Updated: updated}, err
	}
	var deletedFingerprints []string
	if releaseRetained {
		deletedFingerprints, err = releaseRetainedProfileNodesExcept(ctx, tx, profileID, keepNodeIDs)
		if err != nil {
			return appevaluations.StateReleaseResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return appevaluations.StateReleaseResult{}, err
	}
	committed = true
	return appevaluations.StateReleaseResult{Updated: true, DeletedFingerprints: deletedFingerprints}, nil
}

const evaluationTargetSelectSQL = `SELECT id, type, fixed_node_id, exit_node_ids_json, chain_evaluation_mode, test_url,
		       egress_country, egress_country_mode, egress_countries_json, node_source_mode, source_ids_json, protocols_json,
		       name_include_regex, name_exclude_regex, manual_only, candidate_limit, min_evaluation_interval_seconds,
		       last_evaluated_at, config_version, relative_improvement_threshold, absolute_latency_improvement_ms,
		       node_sticky_enabled
		  FROM access_profiles`

type evaluationTargetScanner interface {
	Scan(dest ...any) error
}

func scanEvaluationTarget(row evaluationTargetScanner) (appevaluations.TargetRecord, error) {
	var target appevaluations.TargetRecord
	var exitNodeIDsJSON, egressCountriesJSON, sourceIDsJSON, protocolsJSON string
	var minInterval, absoluteImprovement int64
	err := row.Scan(
		&target.ID,
		&target.Type,
		&target.FixedNodeID,
		&exitNodeIDsJSON,
		&target.ChainEvaluationMode,
		&target.TestURL,
		&target.EgressCountry,
		&target.EgressCountryMode,
		&egressCountriesJSON,
		&target.NodeSourceMode,
		&sourceIDsJSON,
		&protocolsJSON,
		&target.NameIncludeRegex,
		&target.NameExcludeRegex,
		&target.ManualOnly,
		&target.CandidateLimit,
		&minInterval,
		&target.LastEvaluatedAt,
		&target.ConfigVersion,
		&target.RelativeImprovementThreshold,
		&absoluteImprovement,
		&target.NodeStickyEnabled,
	)
	if err != nil {
		return appevaluations.TargetRecord{}, err
	}
	target.MinEvaluationIntervalSeconds = int(minInterval)
	target.AbsoluteImprovementMS = int(absoluteImprovement)
	target.ExitNodeIDs = unmarshalStringSlice(exitNodeIDsJSON)
	target.EgressCountries = unmarshalStringSlice(egressCountriesJSON)
	target.SourceIDs = unmarshalStringSlice(sourceIDsJSON)
	target.Protocols = unmarshalStringSlice(protocolsJSON)
	return target, nil
}

type evaluationProfileStateExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func updateEvaluationProfileState(ctx context.Context, exec evaluationProfileStateExecutor, profileID string, configVersion int64, update appevaluations.StateUpdate) (bool, error) {
	assignments, args, err := evaluationProfileStateAssignments(update)
	if err != nil {
		return false, err
	}
	args = append(args, profileID, configVersion, configVersion)
	query := `UPDATE access_profiles SET ` + strings.Join(assignments, ", ") +
		` WHERE id = $` + placeholder(len(args)-2) +
		` AND (config_version = $` + placeholder(len(args)-1) +
		` OR $` + placeholder(len(args)) + ` = 0)`
	res, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func evaluationProfileStateAssignments(update appevaluations.StateUpdate) ([]string, []any, error) {
	assignments := []string{}
	args := []any{}
	add := func(column string, value any) {
		args = append(args, value)
		assignments = append(assignments, column+" = $"+placeholder(len(args)))
	}
	if update.State != nil {
		add("state", *update.State)
	}
	if update.LastError != nil {
		add("last_error", *update.LastError)
	}
	if update.CurrentNodeID != nil {
		add("current_node_id", *update.CurrentNodeID)
	}
	if update.CurrentExitNodeID != nil {
		add("current_exit_node_id", *update.CurrentExitNodeID)
	}
	if update.CurrentPathLatencyMS != nil {
		add("current_path_latency_ms", *update.CurrentPathLatencyMS)
	}
	if update.CurrentPathFailedEvaluations != nil {
		add("current_path_failed_evaluations", *update.CurrentPathFailedEvaluations)
	} else if update.IncrementCurrentPathCounters {
		assignments = append(assignments, "current_path_failed_evaluations = current_path_failed_evaluations + 1")
	}
	if update.CurrentPathMissedSuccessCycles != nil {
		add("current_path_missed_success_cycles", *update.CurrentPathMissedSuccessCycles)
	} else if update.IncrementCurrentPathCounters {
		assignments = append(assignments, "current_path_missed_success_cycles = current_path_missed_success_cycles + 1")
	}
	if update.SwitchReason != nil {
		add("switch_reason", *update.SwitchReason)
	}
	if update.LastEvaluationDetailsJSON != nil {
		add("last_evaluation_details_json", *update.LastEvaluationDetailsJSON)
	}
	if update.LastEvaluatedAt != nil {
		add("last_evaluated_at", *update.LastEvaluatedAt)
	}
	if update.LastEvaluationStartedAt != nil {
		add("last_evaluation_started_at", *update.LastEvaluationStartedAt)
	}
	if len(assignments) == 0 {
		return nil, nil, errors.New("empty profile state update")
	}
	return assignments, args, nil
}

var _ appevaluations.Repository = EvaluationRepository{}
