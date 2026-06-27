package sqlite

import (
	"context"
	"database/sql"

	appoverview "proxygateway/internal/application/overview"
)

type OverviewRepository struct {
	db *sql.DB
}

func NewOverviewRepository(db *sql.DB) OverviewRepository {
	return OverviewRepository{db: db}
}

func (r OverviewRepository) LoadResourceCounts(ctx context.Context) (appoverview.ResourceCounts, error) {
	var counts appoverview.ResourceCounts
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions`).Scan(&counts.Subscriptions); err != nil {
		return appoverview.ResourceCounts{}, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&counts.Nodes); err != nil {
		return appoverview.ResourceCounts{}, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes n JOIN node_observations o ON n.id = o.node_id WHERE o.usable = 1 AND n.enabled = 1`).Scan(&counts.UsableNodes); err != nil {
		return appoverview.ResourceCounts{}, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM access_profiles`).Scan(&counts.Profiles); err != nil {
		return appoverview.ResourceCounts{}, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxy_credentials`).Scan(&counts.Credentials); err != nil {
		return appoverview.ResourceCounts{}, err
	}
	return counts, nil
}

func (r OverviewRepository) LoadProfileStateCounts(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM access_profiles GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		counts[state] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}
