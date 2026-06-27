package storage

import (
	"database/sql"
	"fmt"

	appadmin "proxygateway/internal/application/admin"
	appdictionaries "proxygateway/internal/application/dictionaries"
	appevaluations "proxygateway/internal/application/evaluations"
	appgeoip "proxygateway/internal/application/geoip"
	appmaintenance "proxygateway/internal/application/maintenance"
	appnodes "proxygateway/internal/application/nodes"
	appobservations "proxygateway/internal/application/observations"
	appoverview "proxygateway/internal/application/overview"
	appprofiles "proxygateway/internal/application/profiles"
	appproxy "proxygateway/internal/application/proxy"
	appsettings "proxygateway/internal/application/settings"
	appsubscriptions "proxygateway/internal/application/subscriptions"
	databaseinfra "proxygateway/internal/infrastructure/database"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
)

type Repositories struct {
	GeoIPStatus       appgeoip.StatusRepository
	KVSettings        appsettings.KVRepository
	SystemSettings    appsettings.SystemRepository
	Admin             appadmin.Repository
	MaintenanceAux    appmaintenance.AuxiliaryRepository
	MaintenanceRun    appmaintenance.Repository
	Overview          appoverview.Repository
	Dictionary        appdictionaries.Repository
	Node              appnodes.Repository
	NodeObservation   appobservations.PersistenceRepository
	Evaluation        appevaluations.Repository
	RequestLog        appproxy.RequestLogRepository
	ProxyCredential   appproxy.CredentialRepository
	ProfileConfig     appprofiles.ConfigRepository
	ProfileCredential appprofiles.CredentialRepository
	Subscription      appsubscriptions.Repository
}

func NewRepositories(handle Handle) (Repositories, error) {
	switch handle.Dialect {
	case "", databaseinfra.DialectSQLite:
		return newSQLiteRepositories(handle.DB), nil
	case databaseinfra.DialectPostgres:
		return Repositories{}, fmt.Errorf("database dialect %q repositories are not implemented yet", handle.Dialect)
	default:
		return Repositories{}, fmt.Errorf("unsupported database dialect %q", handle.Dialect)
	}
}

func newSQLiteRepositories(db *sql.DB) Repositories {
	return Repositories{
		GeoIPStatus:       sqliteinfra.NewGeoIPStatusRepository(db),
		KVSettings:        sqliteinfra.NewKVSettingsRepository(db),
		SystemSettings:    sqliteinfra.NewSystemSettingsRepository(db),
		Admin:             sqliteinfra.NewAdminRepository(db),
		MaintenanceAux:    sqliteinfra.NewMaintenanceAuxiliaryRepository(db),
		MaintenanceRun:    newSQLiteMaintenanceRunRepository(db),
		Overview:          sqliteinfra.NewOverviewRepository(db),
		Dictionary:        sqliteinfra.NewDictionaryRepository(db),
		Node:              sqliteinfra.NewNodeRepository(db),
		NodeObservation:   sqliteinfra.NewNodeObservationRepository(db),
		Evaluation:        sqliteinfra.NewEvaluationRepository(db),
		RequestLog:        sqliteinfra.NewRequestLogRepository(db),
		ProxyCredential:   sqliteinfra.NewProxyCredentialRepository(db),
		ProfileConfig:     sqliteinfra.NewProfileConfigRepository(db),
		ProfileCredential: sqliteinfra.NewProfileCredentialRepository(db),
		Subscription:      sqliteinfra.NewSubscriptionRepository(db),
	}
}
