package app

import (
	"context"
	"sync"

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
	domainprofile "proxygateway/internal/domain/profile"
	geoipinfra "proxygateway/internal/infrastructure/geoip"
	storageinfra "proxygateway/internal/infrastructure/storage"

	"go.uber.org/zap"
)

type Gateway struct {
	store                 storageinfra.Handle
	txRunners             storageinfra.TxRunners
	dataDir               string
	protocolEngine        nodeProtocolEngine
	geoIP                 *geoipinfra.Service
	geoIPStatusRepo       appgeoip.StatusRepository
	maintenance           *maintenanceRunner
	maintenanceAuxRepo    appmaintenance.AuxiliaryRepository
	maintenanceRunRepo    appmaintenance.Repository
	overviewRepo          appoverview.Repository
	dictionaryRepo        appdictionaries.Repository
	nodeRepo              appnodes.Repository
	nodeObservationRepo   appobservations.PersistenceRepository
	evaluationRepo        appevaluations.Repository
	requestLogs           *appproxy.RequestLogWriter
	requestLogService     *appproxy.RequestLogService
	requestLogRepo        appproxy.RequestLogRepository
	proxyCredentialRepo   appproxy.CredentialRepository
	profileConfigRepo     appprofiles.ConfigRepository
	profileCredentialRepo appprofiles.CredentialRepository
	subscriptionRepo      appsubscriptions.Repository
	subscriptionFetcher   appsubscriptions.ContentFetcher
	kvSettingsRepo        appsettings.KVRepository
	systemSettingsRepo    appsettings.SystemRepository
	adminRepo             appadmin.Repository
	adminLogins           *appadmin.LoginLimiter
	ctx                   context.Context
	cancel                context.CancelFunc
	closeOnce             sync.Once
	logger                *zap.Logger
}

type nodeProtocolEngine = appproxy.NodeProtocolEngine
type closeableNodeProtocolEngine = appproxy.CloseableNodeProtocolEngine

type geoIPCountryService interface {
	LookupCountry(ip string) string
	UpdateFromMetaCubeXLatest() error
	Close()
}

type dialTimeouts = appproxy.DialTimeouts
type dialResult = appproxy.DialResult
type dialMetrics = appproxy.DialMetrics
type nodeRecord = appproxy.Node
type proxyCredentialRecord = appproxy.CredentialRecord
type selectedProxyPath = appproxy.SelectedPath

type evaluationSettings struct {
	GlobalConcurrency                   int `json:"global_concurrency"`
	DefaultMinEvaluationIntervalSeconds int `json:"default_min_evaluation_interval_seconds"`
	SingleCandidateLimit                int `json:"single_candidate_limit"`
	ChainCandidateLimit                 int `json:"chain_candidate_limit"`
	ConnectTimeoutSeconds               int `json:"connect_timeout_seconds"`
	ProbeTimeoutSeconds                 int `json:"probe_timeout_seconds"`
}

type maintenanceSettings struct {
	SubscriptionRefreshSeconds   int    `json:"subscription_refresh_seconds"`
	NodeObservationSeconds       int    `json:"node_observation_seconds"`
	ProfileEvaluationSeconds     int    `json:"profile_evaluation_seconds"`
	ChainEvaluationSeconds       int    `json:"chain_evaluation_seconds"`
	GeoIPUpdateTime              string `json:"geoip_update_time"`
	EgressIPProbeURL             string `json:"egress_ip_probe_url"`
	SubscriptionConcurrency      int    `json:"subscription_concurrency"`
	NodeObservationConcurrency   int    `json:"node_observation_concurrency"`
	ProfileEvaluationConcurrency int    `json:"profile_evaluation_concurrency"`
	GeoIPConcurrency             int    `json:"geoip_concurrency"`
}

type profileEvaluationTarget = appevaluations.Target

type parsedSubscriptionNode = appsubscriptions.ParsedNode

type candidateFilter = domainprofile.CandidateFilter
