package postgres

import (
	"database/sql"

	appobservations "proxygateway/internal/application/observations"
)

type NodeObservationRepository struct {
	db *sql.DB
}

func NewNodeObservationRepository(db *sql.DB) NodeObservationRepository {
	return NodeObservationRepository{db: db}
}

func (r NodeObservationRepository) SaveSuccess(nodeID string, record appobservations.SuccessRecord, observedAt int64) error {
	_, err := r.db.Exec(
		`INSERT INTO node_observations (node_id, usable, egress_ip, egress_country, latency_ms, last_error, last_success_at)
		 VALUES ($1, true, $2, $3, $4, '', $5)
		 ON CONFLICT(node_id) DO UPDATE SET
			usable = true,
			egress_ip = excluded.egress_ip,
			egress_country = excluded.egress_country,
			latency_ms = excluded.latency_ms,
			last_error = '',
			last_success_at = excluded.last_success_at`,
		nodeID,
		record.EgressIP,
		record.EgressCountry,
		record.LatencyMS,
		observedAt,
	)
	return err
}

func (r NodeObservationRepository) SaveFailure(nodeID, errorText string, observedAt int64) error {
	_, err := r.db.Exec(
		`INSERT INTO node_observations (node_id, usable, last_error, last_failure_at)
		 VALUES ($1, false, $2, $3)
		 ON CONFLICT(node_id) DO UPDATE SET
			usable = false,
			last_error = excluded.last_error,
			last_failure_at = excluded.last_failure_at`,
		nodeID,
		errorText,
		observedAt,
	)
	return err
}

var _ appobservations.PersistenceRepository = NodeObservationRepository{}
