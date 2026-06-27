package sqlite

import (
	"database/sql"
	"errors"

	appnodes "proxygateway/internal/application/nodes"
)

type NodeUpsertRepositoryTx struct {
	tx *sql.Tx
}

func NewNodeUpsertRepositoryTx(tx *sql.Tx) NodeUpsertRepositoryTx {
	return NodeUpsertRepositoryTx{tx: tx}
}

func (r NodeUpsertRepositoryTx) FindNodeIDByFingerprint(fingerprint string) (string, error) {
	var id string
	err := r.tx.QueryRow(`SELECT id FROM nodes WHERE fingerprint = ?`, fingerprint).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (r NodeUpsertRepositoryTx) CreateNode(record appnodes.CreateNodeRecord) error {
	_, err := r.tx.Exec(
		`INSERT INTO nodes (id, fingerprint, name, type, server, server_port, username, password, raw_json, outbound_json, source_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.Fingerprint,
		record.Name,
		record.Type,
		record.Server,
		record.ServerPort,
		record.Username,
		record.Password,
		record.RawJSON,
		record.OutboundJSON,
		record.SourceID,
		record.CreatedAt,
	)
	return err
}

func (r NodeUpsertRepositoryTx) BindNodeSource(record appnodes.BindSourceRecord) error {
	_, err := r.tx.Exec(
		`INSERT OR IGNORE INTO node_sources (node_id, source_id, source_name, source_type, display_name, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		record.NodeID,
		record.SourceID,
		record.SourceName,
		record.SourceType,
		record.DisplayName,
		record.CreatedAt,
	)
	return err
}

type NodeManualUpdateRepositoryTx struct {
	NodeUpsertRepositoryTx
}

func NewNodeManualUpdateRepositoryTx(tx *sql.Tx) NodeManualUpdateRepositoryTx {
	return NodeManualUpdateRepositoryTx{NodeUpsertRepositoryTx: NewNodeUpsertRepositoryTx(tx)}
}

func (r NodeManualUpdateRepositoryTx) CurrentNodeEnabled(nodeID string) (int, error) {
	var enabled int
	err := r.tx.QueryRow(`SELECT enabled FROM nodes WHERE id = ?`, nodeID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, appnodes.ErrNodeNotFound
	}
	return enabled, err
}

func (r NodeManualUpdateRepositoryTx) NodeSourceCounts(nodeID string) (int, int, error) {
	var manualSources, totalSources int
	err := r.tx.QueryRow(
		`SELECT COALESCE(SUM(CASE WHEN source_type = 'manual' THEN 1 ELSE 0 END), 0), COUNT(*)
		   FROM node_sources
		  WHERE node_id = ?`,
		nodeID,
	).Scan(&manualSources, &totalSources)
	return manualSources, totalSources, err
}

func (r NodeManualUpdateRepositoryTx) FindOtherNodeIDByFingerprint(fingerprint, excludeNodeID string) (string, error) {
	var id string
	err := r.tx.QueryRow(`SELECT id FROM nodes WHERE fingerprint = ? AND id != ? LIMIT 1`, fingerprint, excludeNodeID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (r NodeManualUpdateRepositoryTx) UpdateNode(record appnodes.UpdateNodeRecord) error {
	_, err := r.tx.Exec(
		`UPDATE nodes
		    SET fingerprint = ?, name = ?, type = ?, server = ?, server_port = ?, username = ?, password = ?, raw_json = ?, outbound_json = ?, enabled = ?
		  WHERE id = ?`,
		record.Fingerprint,
		record.Name,
		record.Type,
		record.Server,
		record.ServerPort,
		record.Username,
		record.Password,
		record.RawJSON,
		record.OutboundJSON,
		record.Enabled,
		record.NodeID,
	)
	return err
}

func (r NodeManualUpdateRepositoryTx) UpdateManualSourceDisplayName(nodeID, name string) error {
	_, err := r.tx.Exec(`UPDATE node_sources SET display_name = ? WHERE node_id = ? AND source_type = 'manual'`, name, nodeID)
	return err
}

func (r NodeManualUpdateRepositoryTx) DeleteManualSource(nodeID string) error {
	_, err := r.tx.Exec(`DELETE FROM node_sources WHERE node_id = ? AND source_type = 'manual'`, nodeID)
	return err
}

func (r NodeManualUpdateRepositoryTx) SetNodeEnabled(nodeID string, enabled int) error {
	_, err := r.tx.Exec(`UPDATE nodes SET enabled = ? WHERE id = ?`, enabled, nodeID)
	return err
}

type NodeDeleteRepositoryTx struct {
	tx *sql.Tx
}

func NewNodeDeleteRepositoryTx(tx *sql.Tx) NodeDeleteRepositoryTx {
	return NodeDeleteRepositoryTx{tx: tx}
}

func (r NodeDeleteRepositoryTx) DeleteManualSource(nodeID string) (int64, error) {
	res, err := r.tx.Exec(`DELETE FROM node_sources WHERE node_id = ? AND source_type = 'manual'`, nodeID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

var _ appnodes.UpsertRepository = NodeUpsertRepositoryTx{}
var _ appnodes.ManualUpdateRepository = NodeManualUpdateRepositoryTx{}
