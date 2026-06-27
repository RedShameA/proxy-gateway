package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	appnodes "proxygateway/internal/application/nodes"
)

func ReleaseRetainedProfileNodesExceptTx(tx *sql.Tx, profileID string, keepNodeIDs []string) ([]string, error) {
	return releaseRetainedProfileNodesExcept(context.Background(), tx, profileID, keepNodeIDs)
}

func (r SubscriptionSourceRepositoryTx) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	return cleanupNodesWithoutReferences(context.Background(), r.tx, nodeIDs)
}

func (r ProfileDeleteRepositoryTx) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	return cleanupNodesWithoutReferences(context.Background(), r.tx, nodeIDs)
}

func (r NodeDeleteRepositoryTx) CleanupNodesWithoutReferences(nodeIDs []string) ([]string, error) {
	return cleanupNodesWithoutReferences(context.Background(), r.tx, nodeIDs)
}

func releaseRetainedProfileNodesExcept(ctx context.Context, tx *sql.Tx, profileID string, keepNodeIDs []string) ([]string, error) {
	retainedNodeIDs, err := retainedNodeIDsForProfile(ctx, tx, profileID)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, nodeID := range keepNodeIDs {
		if strings.TrimSpace(nodeID) != "" {
			keep[nodeID] = true
		}
	}
	var releaseNodeIDs []string
	for _, nodeID := range retainedNodeIDs {
		if keep[nodeID] {
			continue
		}
		releaseNodeIDs = append(releaseNodeIDs, nodeID)
	}
	for _, nodeID := range releaseNodeIDs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM retained_profile_nodes WHERE profile_id = ? AND node_id = ?`, profileID, nodeID); err != nil {
			return nil, err
		}
	}
	return cleanupNodesWithoutReferences(ctx, tx, releaseNodeIDs)
}

func retainedNodeIDsForProfile(ctx context.Context, tx *sql.Tx, profileID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT node_id FROM retained_profile_nodes WHERE profile_id = ?`, profileID)
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
	return nodeIDs, rows.Err()
}

func cleanupNodesWithoutReferences(ctx context.Context, tx *sql.Tx, nodeIDs []string) ([]string, error) {
	var deletedFingerprints []string
	for _, nodeID := range nodeIDs {
		var remainingSources int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM node_sources WHERE node_id = ?`, nodeID).Scan(&remainingSources); err != nil {
			return nil, err
		}
		if remainingSources > 0 {
			continue
		}
		var retainedProfiles int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM retained_profile_nodes WHERE node_id = ?`, nodeID).Scan(&retainedProfiles); err != nil {
			return nil, err
		}
		if retainedProfiles > 0 {
			continue
		}
		fingerprint, err := nodeRuntimeFingerprint(ctx, tx, nodeID)
		if err != nil {
			if errors.Is(err, appnodes.ErrNodeNotFound) {
				continue
			}
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM node_observations WHERE node_id = ?`, nodeID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, nodeID); err != nil {
			return nil, err
		}
		deletedFingerprints = append(deletedFingerprints, fingerprint)
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE access_profiles
			    SET current_node_id = '',
			        state = 'invalid_config',
			        last_error = 'referenced node no longer exists',
			        switch_reason = 'missing_fixed_node'
			  WHERE fixed_node_id = ?`,
			nodeID,
		); err != nil {
			return nil, err
		}
		if err := resetDynamicProfilesForRemovedCurrentNode(ctx, tx, nodeID); err != nil {
			return nil, err
		}
	}
	return deletedFingerprints, nil
}

func nodeRuntimeFingerprint(ctx context.Context, tx *sql.Tx, nodeID string) (string, error) {
	var fingerprint, outboundJSON string
	err := tx.QueryRowContext(ctx, `SELECT fingerprint, outbound_json FROM nodes WHERE id = ?`, nodeID).Scan(&fingerprint, &outboundJSON)
	if err == sql.ErrNoRows {
		return "", appnodes.ErrNodeNotFound
	}
	if err != nil {
		return "", err
	}
	if trimmed := strings.TrimSpace(outboundJSON); trimmed != "" {
		return appnodes.OutboundFingerprint(trimmed), nil
	}
	return fingerprint, nil
}

func resetDynamicProfilesForRemovedCurrentNode(ctx context.Context, tx *sql.Tx, nodeID string) error {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT id, auto_evaluation_enabled
		   FROM access_profiles
		  WHERE current_node_id = ?
		    AND fixed_node_id != ?
		    AND state != 'invalid_config'
		    AND type IN ('fastest', 'chain')`,
		nodeID,
		nodeID,
	)
	if err != nil {
		return err
	}
	type profileRef struct {
		id       string
		autoEval bool
	}
	var profiles []profileRef
	for rows.Next() {
		var profile profileRef
		var autoEval int
		if err := rows.Scan(&profile.id, &autoEval); err != nil {
			_ = rows.Close()
			return err
		}
		profile.autoEval = autoEval == 1
		profiles = append(profiles, profile)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	nowMillis := time.Now().UnixMilli()
	for _, profile := range profiles {
		if profile.autoEval {
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE access_profiles
				    SET current_node_id = '',
				        current_exit_node_id = CASE WHEN current_exit_node_id = ? THEN '' ELSE current_exit_node_id END,
				        state = 'waiting_observation',
				        last_error = '',
				        switch_reason = 'current_node_removed',
				        last_evaluation_started_at = ?
				  WHERE id = ?`,
				nodeID,
				nowMillis,
				profile.id,
			); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE access_profiles
			    SET current_node_id = '',
			        current_exit_node_id = CASE WHEN current_exit_node_id = ? THEN '' ELSE current_exit_node_id END,
			        state = 'pending',
			        last_error = 'current node no longer exists',
			        switch_reason = 'current_node_removed'
			  WHERE id = ?`,
			nodeID,
			profile.id,
		); err != nil {
			return err
		}
	}
	return nil
}
