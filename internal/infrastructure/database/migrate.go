package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"
)

const VersionTable = "goose_db_version"

type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

type MigrationSet struct {
	Dialect      Dialect
	Files        fs.FS
	GoMigrations []*goose.Migration
}

func Migrate(ctx context.Context, db *sql.DB, set MigrationSet) error {
	if db == nil {
		return errors.New("database handle is nil")
	}
	dialect, err := set.Dialect.gooseDialect()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(
		dialect,
		db,
		set.Files,
		goose.WithGoMigrations(set.GoMigrations...),
		goose.WithDisableGlobalRegistry(true),
		goose.WithTableName(VersionTable),
	)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("run %s migrations: %w", set.Dialect, err)
	}
	return nil
}

func (d Dialect) gooseDialect() (goose.Dialect, error) {
	switch d {
	case DialectSQLite:
		return goose.DialectSQLite3, nil
	case DialectPostgres:
		return goose.DialectPostgres, nil
	default:
		return "", fmt.Errorf("unsupported database dialect %q", d)
	}
}
