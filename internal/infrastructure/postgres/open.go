package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const DriverName = "pgx"

func Open(dsn string) (*sql.DB, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("postgres DSN is empty")
	}
	db, err := sql.Open(DriverName, dsn)
	if err != nil {
		return nil, errors.New("open postgres database failed")
	}
	ConfigureConnection(db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect postgres database failed: %s", sanitizeConnectionError(err))
	}
	return db, nil
}

func ConfigureConnection(db *sql.DB) {
	if db == nil {
		return
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
}

func sanitizeConnectionError(err error) string {
	if err == nil {
		return ""
	}
	// pgx connection errors can contain host/user details. Keep the returned error
	// useful for startup diagnosis without ever reflecting the DSN or password.
	return "connection unavailable"
}
