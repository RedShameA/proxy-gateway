package sqlite

import "database/sql"

type ProfileDeleteRepositoryTx struct {
	tx *sql.Tx
}

func NewProfileDeleteRepositoryTx(tx *sql.Tx) ProfileDeleteRepositoryTx {
	return ProfileDeleteRepositoryTx{tx: tx}
}

func (r ProfileDeleteRepositoryTx) DeleteCredentials(profileID string) error {
	_, err := r.tx.Exec(`DELETE FROM proxy_credentials WHERE profile_id = ?`, profileID)
	return err
}

func (r ProfileDeleteRepositoryTx) DeleteMaintenanceRuns(profileID string) error {
	_, err := r.tx.Exec(`DELETE FROM maintenance_runs WHERE target_id = ?`, profileID)
	return err
}

func (r ProfileDeleteRepositoryTx) RetainedNodeIDs(profileID string) ([]string, error) {
	rows, err := r.tx.Query(`SELECT node_id FROM retained_profile_nodes WHERE profile_id = ?`, profileID)
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

func (r ProfileDeleteRepositoryTx) DeleteRetainedNodes(profileID string) error {
	_, err := r.tx.Exec(`DELETE FROM retained_profile_nodes WHERE profile_id = ?`, profileID)
	return err
}

func (r ProfileDeleteRepositoryTx) DeleteProfile(profileID string) (int64, error) {
	result, err := r.tx.Exec(`DELETE FROM access_profiles WHERE id = ?`, profileID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
