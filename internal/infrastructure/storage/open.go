package storage

import (
	"database/sql"
	"fmt"
	"strings"

	databaseinfra "proxygateway/internal/infrastructure/database"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
)

const DriverSQLite = "sqlite"

type Config struct {
	Driver     string
	DataDir    string
	SQLitePath string
	DSN        string
}

type Handle struct {
	DB      *sql.DB
	Dialect databaseinfra.Dialect
}

func Open(config Config) (Handle, error) {
	driver := strings.TrimSpace(config.Driver)
	if driver == "" {
		driver = DriverSQLite
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
	case string(databaseinfra.DialectPostgres), "postgresql":
		return Handle{}, fmt.Errorf("database driver %q is not supported yet", driver)
	default:
		return Handle{}, fmt.Errorf("unsupported database driver %q", driver)
	}
}
