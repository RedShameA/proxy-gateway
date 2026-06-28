package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	databaseinfra "proxygateway/internal/infrastructure/database"
	postgresinfra "proxygateway/internal/infrastructure/postgres"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
)

const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"

	EnvDBDriver = "PROXYGATEWAY_DB_DRIVER"
	EnvDBDSN    = "PROXYGATEWAY_DB_DSN"
)

type Config struct {
	Driver     string
	DataDir    string
	SQLitePath string
	DSN        string
}

func ConfigFromEnv(dataDir string, lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("environment lookup is nil")
	}
	rawDriver, driverSet := lookup(EnvDBDriver)
	dsn, dsnSet := lookup(EnvDBDSN)
	driver, err := NormalizeDriver(rawDriver)
	if err != nil {
		return Config{}, err
	}
	if !driverSet {
		if dsnSet && strings.TrimSpace(dsn) != "" {
			return Config{}, fmt.Errorf("%s must be set when %s is configured", EnvDBDriver, EnvDBDSN)
		}
		driver = DriverSQLite
	}
	if driver == DriverSQLite {
		dsn = ""
	}
	return Config{
		Driver:  driver,
		DataDir: dataDir,
		DSN:     strings.TrimSpace(dsn),
	}, nil
}

func NormalizeDriver(driver string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(driver)); normalized {
	case "", DriverSQLite:
		return DriverSQLite, nil
	case DriverPostgres, "postgresql":
		return DriverPostgres, nil
	default:
		return "", fmt.Errorf("unsupported database driver %q", normalized)
	}
}

func Migrate(ctx context.Context, handle Handle) error {
	switch handle.Dialect {
	case "", databaseinfra.DialectSQLite:
		return sqliteinfra.Migrate(ctx, handle.DB)
	case databaseinfra.DialectPostgres:
		return postgresinfra.Migrate(ctx, handle.DB)
	default:
		return fmt.Errorf("unsupported database dialect %q", handle.Dialect)
	}
}

type Handle struct {
	DB      *sql.DB
	Dialect databaseinfra.Dialect
}

func Open(config Config) (Handle, error) {
	driver, err := NormalizeDriver(config.Driver)
	if err != nil {
		return Handle{}, err
	}
	switch driver {
	case DriverSQLite:
		path := strings.TrimSpace(config.SQLitePath)
		if path == "" {
			path = sqliteinfra.DefaultPath(config.DataDir)
		}
		db, err := sqliteinfra.Open(path)
		if err != nil {
			return Handle{}, err
		}
		return Handle{DB: db, Dialect: databaseinfra.DialectSQLite}, nil
	case DriverPostgres:
		db, err := postgresinfra.Open(config.DSN)
		if err != nil {
			return Handle{}, err
		}
		return Handle{DB: db, Dialect: databaseinfra.DialectPostgres}, nil
	default:
		return Handle{}, fmt.Errorf("unsupported database driver %q", driver)
	}
}
