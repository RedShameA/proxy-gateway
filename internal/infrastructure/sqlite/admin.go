package sqlite

import (
	"context"
	"database/sql"

	appadmin "proxygateway/internal/application/admin"
)

type AdminRepository struct {
	db *sql.DB
}

func NewAdminRepository(db *sql.DB) AdminRepository {
	return AdminRepository{db: db}
}

func (r AdminRepository) HasCredential(ctx context.Context) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM admin_credentials WHERE id = 1`).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (r AdminRepository) CreateCredential(ctx context.Context, record appadmin.CredentialRecord) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO admin_credentials (id, password_hash, created_at) VALUES (1, ?, ?)`,
		record.PasswordHash,
		record.CreatedAt,
	)
	return err
}

func (r AdminRepository) LoadPasswordHash(ctx context.Context) (string, bool, error) {
	var hash string
	err := r.db.QueryRowContext(ctx, `SELECT password_hash FROM admin_credentials LIMIT 1`).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}

func (r AdminRepository) CreateSession(ctx context.Context, record appadmin.SessionRecord) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO admin_sessions (token_hash, created_at) VALUES (?, ?)`,
		record.TokenHash,
		record.CreatedAt,
	)
	return err
}

func (r AdminRepository) LoadSessionCreatedAt(ctx context.Context, tokenHash string) (int64, bool, error) {
	var createdAt int64
	err := r.db.QueryRowContext(ctx, `SELECT created_at FROM admin_sessions WHERE token_hash = ?`, tokenHash).Scan(&createdAt)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return createdAt, true, nil
}

func (r AdminRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (r AdminRepository) DeleteExpiredSessions(ctx context.Context, cutoffMillis int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE created_at <= ?`, cutoffMillis)
	return err
}

func (r AdminRepository) UpdatePasswordAndDeleteSessions(ctx context.Context, passwordHash string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(ctx, `UPDATE admin_credentials SET password_hash = ?`, passwordHash); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_sessions`); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
