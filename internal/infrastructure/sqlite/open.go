package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	DriverName      = "sqlite"
	DefaultFileName = "app.db"
)

func DefaultPath(dataDir string) string {
	return filepath.Join(dataDir, DefaultFileName)
}

func Open(path string) (*sql.DB, error) {
	if path == "" {
		return nil, errors.New("sqlite database path is empty")
	}
	db, err := sql.Open(DriverName, path)
	if err != nil {
		return nil, err
	}
	if err := ConfigureConnection(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ConfigureConnection(db *sql.DB) error {
	if db == nil {
		return errors.New("database handle is nil")
	}
	// Keep SQLite writes serialized. Raising this requires per-connection PRAGMA setup and lock testing.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err := db.Exec(`PRAGMA busy_timeout = 5000`)
	return err
}
