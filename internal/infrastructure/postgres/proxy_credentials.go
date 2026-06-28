package postgres

import (
	"context"
	"database/sql"

	appproxy "proxygateway/internal/application/proxy"
)

type ProxyCredentialRepository struct {
	db *sql.DB
}

func NewProxyCredentialRepository(db *sql.DB) ProxyCredentialRepository {
	return ProxyCredentialRepository{db: db}
}

func (r ProxyCredentialRepository) LookupCredential(ctx context.Context, profileIdentifier, password string) (appproxy.CredentialRecord, bool, bool, error) {
	var profileID string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM access_profiles WHERE profile_identifier = $1`, profileIdentifier).Scan(&profileID)
	if err == sql.ErrNoRows {
		return appproxy.CredentialRecord{}, false, false, nil
	}
	if err != nil {
		return appproxy.CredentialRecord{}, false, false, err
	}

	var record appproxy.CredentialRecord
	err = r.db.QueryRowContext(
		ctx,
		`SELECT c.id, c.remark, c.profile_id
		   FROM proxy_credentials c
		  WHERE c.profile_id = $1
		    AND c.password = $2
		    AND c.enabled = true`,
		profileID,
		password,
	).Scan(&record.ID, &record.Remark, &record.ProfileID)
	if err == sql.ErrNoRows {
		return appproxy.CredentialRecord{}, true, false, nil
	}
	if err != nil {
		return appproxy.CredentialRecord{}, true, false, err
	}
	return record, true, true, nil
}

func (r ProxyCredentialRepository) TouchCredentialLastUsed(ctx context.Context, credentialID string, nowMillis, cutoffMillis int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE proxy_credentials SET last_used_at = $1 WHERE id = $2 AND last_used_at < $3`, nowMillis, credentialID, cutoffMillis)
	return err
}

var _ appproxy.CredentialRepository = ProxyCredentialRepository{}
