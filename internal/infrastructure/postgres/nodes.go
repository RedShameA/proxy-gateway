package postgres

import (
	"context"
	"database/sql"
	"strings"

	appnodes "proxygateway/internal/application/nodes"
)

type NodeRepository struct {
	db *sql.DB
}

func NewNodeRepository(db *sql.DB) NodeRepository {
	return NodeRepository{db: db}
}

func (r NodeRepository) Load(ctx context.Context, id string) (appnodes.Record, bool, error) {
	record, err := scanNodeRecord(r.db.QueryRowContext(
		ctx,
		`SELECT id, name, type, server, server_port, username, password, raw_json, outbound_json, enabled
		   FROM nodes
		  WHERE id = $1`,
		id,
	))
	if err == sql.ErrNoRows {
		return appnodes.Record{}, false, nil
	}
	if err != nil {
		return appnodes.Record{}, false, err
	}
	return record, true, nil
}

func (r NodeRepository) ListIDs(ctx context.Context, filter appnodes.ListFilter) (appnodes.ListResult, error) {
	where, args := nodeListWhere(filter)
	from := ` FROM nodes n LEFT JOIN node_observations o ON o.node_id = n.id`
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+from+where, args...).Scan(&total); err != nil {
		return appnodes.ListResult{}, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT n.id`+from+where+` ORDER BY n.created_at, n.id LIMIT $`+placeholder(len(args)-1)+` OFFSET $`+placeholder(len(args)),
		args...,
	)
	if err != nil {
		return appnodes.ListResult{}, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return appnodes.ListResult{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return appnodes.ListResult{}, err
	}
	return appnodes.ListResult{IDs: ids, Total: total}, nil
}

func (r NodeRepository) ListEnabledObservationTargets(ctx context.Context) ([]appnodes.Record, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, name, type, server, server_port, username, password, raw_json, outbound_json, enabled
		   FROM nodes
		  WHERE enabled = true
		  ORDER BY created_at, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []appnodes.Record{}
	for rows.Next() {
		record, err := scanNodeRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r NodeRepository) ListSources(ctx context.Context, nodeID string) ([]appnodes.SourceRecord, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT source_id, source_name, source_type, display_name
		   FROM node_sources
		  WHERE node_id = $1
		  ORDER BY created_at, source_id`,
		nodeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []appnodes.SourceRecord{}
	for rows.Next() {
		var source appnodes.SourceRecord
		if err := rows.Scan(&source.SourceID, &source.SourceName, &source.SourceType, &source.DisplayName); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sources, nil
}

func (r NodeRepository) LoadObservation(ctx context.Context, nodeID string) (appnodes.ObservationRecord, bool, error) {
	var record appnodes.ObservationRecord
	err := r.db.QueryRowContext(
		ctx,
		`SELECT usable, egress_ip, egress_country, latency_ms, last_error, last_success_at, last_failure_at
		   FROM node_observations
		  WHERE node_id = $1`,
		nodeID,
	).Scan(&record.Usable, &record.EgressIP, &record.EgressCountry, &record.LatencyMS, &record.LastError, &record.LastSuccessAt, &record.LastFailureAt)
	if err == sql.ErrNoRows {
		return appnodes.ObservationRecord{}, false, nil
	}
	if err != nil {
		return appnodes.ObservationRecord{}, false, err
	}
	return record, true, nil
}

func (r NodeRepository) SetEnabled(ctx context.Context, nodeID string, enabled bool) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE nodes SET enabled = $1 WHERE id = $2`, enabled, nodeID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func nodeListWhere(filter appnodes.ListFilter) (string, []any) {
	clauses := []string{`NOT (
		NOT EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id)
		AND EXISTS (SELECT 1 FROM retained_profile_nodes rp WHERE rp.node_id = n.id)
	)`}
	args := []any{}
	addClause := func(clause string, values ...any) {
		for _, value := range values {
			args = append(args, value)
			clause = strings.Replace(clause, "?", "$"+placeholder(len(args)), 1)
		}
		clauses = append(clauses, clause)
	}
	if value := strings.ToLower(strings.TrimSpace(filter.Name)); value != "" {
		pattern := likeContainsPattern(value)
		addClause(`(LOWER(n.name) LIKE ? ESCAPE '\' OR EXISTS (
			SELECT 1 FROM node_sources s
			 WHERE s.node_id = n.id
			   AND (LOWER(s.source_name) LIKE ? ESCAPE '\' OR LOWER(s.display_name) LIKE ? ESCAPE '\')
		))`, pattern, pattern, pattern)
	}
	if country := strings.TrimSpace(filter.EgressCountry); country != "" {
		addClause(`CASE WHEN TRIM(COALESCE(o.egress_country, '')) = '' THEN '__unknown__' ELSE UPPER(o.egress_country) END = ?`, country)
	}
	if protocol := strings.ToLower(strings.TrimSpace(filter.Protocol)); protocol != "" {
		addClause(`LOWER(n.type) = ?`, protocol)
	}
	if sourceID := strings.TrimSpace(filter.SourceID); sourceID != "" {
		addClause(`EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id AND s.source_id = ?)`, sourceID)
	}
	if sourceType := strings.TrimSpace(filter.SourceType); sourceType != "" {
		addClause(`EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id AND s.source_type = ?)`, sourceType)
	}
	switch strings.ToLower(strings.TrimSpace(filter.State)) {
	case appnodes.StateDisabled:
		clauses = append(clauses, `n.enabled != true`)
	case appnodes.StatePendingObservation:
		clauses = append(clauses, `n.enabled = true AND o.node_id IS NULL`)
	case appnodes.StateUsable:
		clauses = append(clauses, `n.enabled = true AND COALESCE(o.usable, false) = true`)
	case appnodes.StateUnusable:
		clauses = append(clauses, `n.enabled = true AND o.node_id IS NOT NULL AND COALESCE(o.usable, false) != true`)
	}
	if filter.Usable != nil {
		if *filter.Usable {
			clauses = append(clauses, `COALESCE(o.usable, false) = true`)
		} else {
			clauses = append(clauses, `COALESCE(o.usable, false) != true`)
		}
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanNodeRecord(row nodeRecordScanner) (appnodes.Record, error) {
	var record appnodes.Record
	err := row.Scan(
		&record.ID,
		&record.Name,
		&record.Type,
		&record.Server,
		&record.ServerPort,
		&record.Username,
		&record.Password,
		&record.RawJSON,
		&record.OutboundJSON,
		&record.Enabled,
	)
	return record, err
}

type nodeRecordScanner interface {
	Scan(dest ...any) error
}

func likeContainsPattern(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(value) + "%"
}

var _ appnodes.Repository = NodeRepository{}
