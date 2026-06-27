package sqlite

import (
	"database/sql"
	"encoding/json"

	appsubscriptions "proxygateway/internal/application/subscriptions"
	domainprofile "proxygateway/internal/domain/profile"
)

type SubscriptionImportRepositoryTx struct {
	tx *sql.Tx
}

func NewSubscriptionImportRepositoryTx(tx *sql.Tx) SubscriptionImportRepositoryTx {
	return SubscriptionImportRepositoryTx{tx: tx}
}

func (r SubscriptionImportRepositoryTx) CreateImport(record appsubscriptions.ImportRecord) error {
	_, err := r.tx.Exec(
		`INSERT INTO subscriptions (
			id, name, source_type, url, content, imported_nodes, skipped_entries, skipped_summary_json,
			auto_refresh_enabled, auto_refresh_interval_seconds, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.Name,
		record.SourceType,
		record.URL,
		record.Content,
		record.ImportedNodes,
		record.SkippedEntries,
		record.SkippedSummaryJSON,
		sqliteBool(record.AutoRefreshEnabled),
		record.AutoRefreshIntervalSeconds,
		record.NowMillis,
		record.NowMillis,
	)
	return err
}

func (r SubscriptionImportRepositoryTx) RefreshImport(record appsubscriptions.ImportRecord) error {
	_, err := r.tx.Exec(
		`UPDATE subscriptions
		    SET content = ?,
		        imported_nodes = ?,
		        skipped_entries = ?,
		        skipped_summary_json = ?,
		        last_error = '',
		        updated_at = ?
		  WHERE id = ?`,
		record.Content,
		record.ImportedNodes,
		record.SkippedEntries,
		record.SkippedSummaryJSON,
		record.NowMillis,
		record.ID,
	)
	return err
}

type SubscriptionSourceRepositoryTx struct {
	tx        *sql.Tx
	nowMillis int64
}

func NewSubscriptionSourceRepositoryTx(tx *sql.Tx, nowMillis int64) SubscriptionSourceRepositoryTx {
	return SubscriptionSourceRepositoryTx{tx: tx, nowMillis: nowMillis}
}

func (r SubscriptionSourceRepositoryTx) ExistingSourceNodeIDs(subscriptionID string) ([]string, error) {
	return r.NodeIDsForSource(subscriptionID, "subscription")
}

func (r SubscriptionSourceRepositoryTx) NodeIDsForSource(sourceID, sourceType string) ([]string, error) {
	rows, err := r.tx.Query(`SELECT node_id FROM node_sources WHERE source_id = ? AND source_type = ?`, sourceID, sourceType)
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

func (r SubscriptionSourceRepositoryTx) DeleteSubscriptionNodeSource(nodeID, subscriptionID string) error {
	_, err := r.tx.Exec(`DELETE FROM node_sources WHERE node_id = ? AND source_id = ? AND source_type = 'subscription'`, nodeID, subscriptionID)
	return err
}

func (r SubscriptionSourceRepositoryTx) DeleteSubscriptionNodeSources(subscriptionID string) error {
	_, err := r.tx.Exec(`DELETE FROM node_sources WHERE source_id = ? AND source_type = 'subscription'`, subscriptionID)
	return err
}

func (r SubscriptionSourceRepositoryTx) DeleteSubscription(subscriptionID string) (int64, error) {
	res, err := r.tx.Exec(`DELETE FROM subscriptions WHERE id = ?`, subscriptionID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r SubscriptionSourceRepositoryTx) RetainStickyProfilesForRemovedNode(nodeID string) ([]appsubscriptions.StickyProfileEvaluationRef, error) {
	rows, err := r.tx.Query(
		`SELECT id, name, config_version
		   FROM access_profiles
		  WHERE node_sticky_enabled = 1
		    AND type IN ('fastest', 'chain')
		    AND state != 'invalid_config'
		    AND (current_node_id = ? OR current_exit_node_id = ?)`,
		nodeID,
		nodeID,
	)
	if err != nil {
		return nil, err
	}
	var profiles []appsubscriptions.StickyProfileEvaluationRef
	for rows.Next() {
		var profile appsubscriptions.StickyProfileEvaluationRef
		if err := rows.Scan(&profile.ID, &profile.Name, &profile.ConfigVersion); err != nil {
			_ = rows.Close()
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if _, err := r.tx.Exec(
			`INSERT OR IGNORE INTO retained_profile_nodes (profile_id, node_id, created_at) VALUES (?, ?, ?)`,
			profile.ID,
			nodeID,
			r.nowMillis,
		); err != nil {
			return nil, err
		}
		if _, err := r.tx.Exec(
			`UPDATE access_profiles
			    SET state = 'degraded',
			        last_error = 'current node no longer exists',
			        switch_reason = 'current_node_removed',
			        last_evaluation_started_at = ?
			  WHERE id = ?`,
			r.nowMillis,
			profile.ID,
		); err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

func (r SubscriptionSourceRepositoryTx) RetainedStickyProfilesForRefresh() ([]appsubscriptions.StickyProfileEvaluationRef, error) {
	rows, err := r.tx.Query(
		`SELECT DISTINCT p.id, p.name, p.config_version
		   FROM access_profiles p
		   JOIN retained_profile_nodes r ON r.profile_id = p.id
		  WHERE p.node_sticky_enabled = 1
		    AND p.type IN ('fastest', 'chain')
		    AND p.state != 'invalid_config'
		  ORDER BY p.created_at, p.id`,
	)
	if err != nil {
		return nil, err
	}
	var profiles []appsubscriptions.StickyProfileEvaluationRef
	for rows.Next() {
		var profile appsubscriptions.StickyProfileEvaluationRef
		if err := rows.Scan(&profile.ID, &profile.Name, &profile.ConfigVersion); err != nil {
			_ = rows.Close()
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if _, err := r.tx.Exec(
			`UPDATE access_profiles
			    SET state = 'degraded',
			        last_error = 'current node no longer exists',
			        switch_reason = 'current_node_removed',
			        last_evaluation_started_at = ?
			  WHERE id = ?`,
			r.nowMillis,
			profile.ID,
		); err != nil {
			return nil, err
		}
	}
	return profiles, nil
}

func (r SubscriptionSourceRepositoryTx) InvalidateProfilesForDeletedSubscription(subscriptionID string) error {
	rows, err := r.tx.Query(`SELECT id, node_source_mode, source_ids_json, manual_only FROM access_profiles`)
	if err != nil {
		return err
	}
	type profileRef struct {
		id string
	}
	var invalid []profileRef
	for rows.Next() {
		var id, nodeSourceMode, sourceIDsJSON string
		var manualOnly int
		if err := rows.Scan(&id, &nodeSourceMode, &sourceIDsJSON, &manualOnly); err != nil {
			_ = rows.Close()
			return err
		}
		var sourceIDs []string
		_ = json.Unmarshal([]byte(sourceIDsJSON), &sourceIDs)
		if domainprofile.NormalizeNodeSourceMode(nodeSourceMode, sourceIDs, manualOnly == 1) != "specific_subscriptions" {
			continue
		}
		for _, sourceID := range sourceIDs {
			if sourceID == subscriptionID {
				invalid = append(invalid, profileRef{id: id})
				break
			}
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, profile := range invalid {
		if _, err := r.tx.Exec(`UPDATE access_profiles SET current_node_id = '', state = 'invalid_config' WHERE id = ?`, profile.id); err != nil {
			return err
		}
	}
	return nil
}

func sqliteBool(value bool) int {
	if value {
		return 1
	}
	return 0
}

var _ appsubscriptions.ImportRepository = SubscriptionImportRepositoryTx{}
