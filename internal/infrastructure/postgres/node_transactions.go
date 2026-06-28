package postgres

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
	err := r.tx.QueryRow(`SELECT id FROM nodes WHERE fingerprint = $1`, fingerprint).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (r NodeUpsertRepositoryTx) CreateNode(record appnodes.CreateNodeRecord) error {
	var id string
	err := r.tx.QueryRow(
		`INSERT INTO nodes (id, fingerprint, name, type, server, server_port, username, password, raw_json, outbound_json, source_id, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 RETURNING id`,
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
	).Scan(&id)
	return err
}

func (r NodeUpsertRepositoryTx) BindNodeSource(record appnodes.BindSourceRecord) error {
	_, err := r.tx.Exec(
		`INSERT INTO node_sources (node_id, source_id, source_name, source_type, display_name, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT(node_id, source_id) DO NOTHING`,
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
	var enabled bool
	err := r.tx.QueryRow(`SELECT enabled FROM nodes WHERE id = $1`, nodeID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, appnodes.ErrNodeNotFound
	}
	if enabled {
		return 1, err
	}
	return 0, err
}

func (r NodeManualUpdateRepositoryTx) NodeSourceCounts(nodeID string) (int, int, error) {
	var manualSources, totalSources int
	err := r.tx.QueryRow(
		`SELECT COALESCE(SUM(CASE WHEN source_type = 'manual' THEN 1 ELSE 0 END), 0), COUNT(*)
		   FROM node_sources
		  WHERE node_id = $1`,
		nodeID,
	).Scan(&manualSources, &totalSources)
	return manualSources, totalSources, err
}

func (r NodeManualUpdateRepositoryTx) FindOtherNodeIDByFingerprint(fingerprint, excludeNodeID string) (string, error) {
	var id string
	err := r.tx.QueryRow(`SELECT id FROM nodes WHERE fingerprint = $1 AND id != $2 LIMIT 1`, fingerprint, excludeNodeID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (r NodeManualUpdateRepositoryTx) UpdateNode(record appnodes.UpdateNodeRecord) error {
	_, err := r.tx.Exec(
		`UPDATE nodes
		    SET fingerprint = $1, name = $2, type = $3, server = $4, server_port = $5, username = $6, password = $7, raw_json = $8, outbound_json = $9, enabled = $10
		  WHERE id = $11`,
		record.Fingerprint,
		record.Name,
		record.Type,
		record.Server,
		record.ServerPort,
		record.Username,
		record.Password,
		record.RawJSON,
		record.OutboundJSON,
		record.Enabled == 1,
		record.NodeID,
	)
	return err
}

func (r NodeManualUpdateRepositoryTx) UpdateManualSourceDisplayName(nodeID, name string) error {
	_, err := r.tx.Exec(`UPDATE node_sources SET display_name = $1 WHERE node_id = $2 AND source_type = 'manual'`, name, nodeID)
	return err
}

func (r NodeManualUpdateRepositoryTx) DeleteManualSource(nodeID string) error {
	_, err := r.tx.Exec(`DELETE FROM node_sources WHERE node_id = $1 AND source_type = 'manual'`, nodeID)
	return err
}

func (r NodeManualUpdateRepositoryTx) SetNodeEnabled(nodeID string, enabled int) error {
	_, err := r.tx.Exec(`UPDATE nodes SET enabled = $1 WHERE id = $2`, enabled == 1, nodeID)
	return err
}

type NodeDeleteRepositoryTx struct {
	tx *sql.Tx
}

func NewNodeDeleteRepositoryTx(tx *sql.Tx) NodeDeleteRepositoryTx {
	return NodeDeleteRepositoryTx{tx: tx}
}

func (r NodeDeleteRepositoryTx) DeleteManualSource(nodeID string) (int64, error) {
	res, err := r.tx.Exec(`DELETE FROM node_sources WHERE node_id = $1 AND source_type = 'manual'`, nodeID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

var _ appnodes.UpsertRepository = NodeUpsertRepositoryTx{}
var _ appnodes.ManualUpdateRepository = NodeManualUpdateRepositoryTx{}
