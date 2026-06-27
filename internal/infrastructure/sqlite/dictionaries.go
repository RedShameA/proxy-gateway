package sqlite

import (
	"context"
	"database/sql"
)

type DictionaryRepository struct {
	db *sql.DB
}

func NewDictionaryRepository(db *sql.DB) DictionaryRepository {
	return DictionaryRepository{db: db}
}

func (r DictionaryRepository) ListEgressCountries(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT egress_country FROM node_observations WHERE egress_country != '' ORDER BY egress_country`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var countries []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		countries = append(countries, code)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return countries, nil
}
