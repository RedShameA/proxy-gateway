package postgres

import (
	"context"
	"database/sql"

	appprofiles "proxygateway/internal/application/profiles"
)

type ProfileCredentialRepository struct {
	db *sql.DB
}

func NewProfileCredentialRepository(db *sql.DB) ProfileCredentialRepository {
	return ProfileCredentialRepository{db: db}
}

func (r ProfileCredentialRepository) ProfileExists(ctx context.Context, profileID string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM access_profiles WHERE id = $1`, profileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (r ProfileCredentialRepository) LoadProfileIdentifier(ctx context.Context, profileID string) (string, bool, error) {
	var identifier string
	err := r.db.QueryRowContext(ctx, `SELECT profile_identifier FROM access_profiles WHERE id = $1`, profileID).Scan(&identifier)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return identifier, true, nil
}

func (r ProfileCredentialRepository) PasswordExists(ctx context.Context, profileID, password string) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM proxy_credentials WHERE profile_id = $1 AND password = $2`, profileID, password).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (r ProfileCredentialRepository) CreateCredential(ctx context.Context, record appprofiles.CredentialCreateRecord) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO proxy_credentials (id, profile_id, remark, password, password_hash, created_at)
		 VALUES ($1, $2, $3, $4, '', $5)`,
		record.ID,
		record.ProfileID,
		record.Remark,
		record.Password,
		record.CreatedAt,
	)
	return err
}

func (r ProfileCredentialRepository) ListCredentials(ctx context.Context, profileID string) ([]appprofiles.CredentialRecord, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, profile_id, remark, password, enabled, created_at, last_used_at
		   FROM proxy_credentials
		  WHERE profile_id = $1
		  ORDER BY created_at, id`,
		profileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []appprofiles.CredentialRecord{}
	for rows.Next() {
		var record appprofiles.CredentialRecord
		if err := rows.Scan(&record.ID, &record.ProfileID, &record.Remark, &record.Password, &record.Enabled, &record.CreatedAt, &record.LastUsedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r ProfileCredentialRepository) SetCredentialEnabled(ctx context.Context, profileID, credentialID string, enabled bool) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE proxy_credentials SET enabled = $1 WHERE id = $2 AND profile_id = $3`, enabled, credentialID, profileID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r ProfileCredentialRepository) DeleteCredential(ctx context.Context, profileID, credentialID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM proxy_credentials WHERE id = $1 AND profile_id = $2`, credentialID, profileID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r ProfileCredentialRepository) CountCredentials(ctx context.Context, profileID string) (appprofiles.CredentialCounts, error) {
	var counts appprofiles.CredentialCounts
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxy_credentials WHERE profile_id = $1`, profileID).Scan(&counts.Total); err != nil {
		return appprofiles.CredentialCounts{}, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxy_credentials WHERE profile_id = $1 AND enabled = true`, profileID).Scan(&counts.Enabled); err != nil {
		return appprofiles.CredentialCounts{}, err
	}
	return counts, nil
}

var _ appprofiles.CredentialRepository = ProfileCredentialRepository{}
