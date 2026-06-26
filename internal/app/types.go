package app

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"sync"
	"time"

	databaseinfra "proxygateway/internal/infrastructure/database"
	geoipinfra "proxygateway/internal/infrastructure/geoip"
)

type Gateway struct {
	db             *sql.DB
	dbDialect      databaseinfra.Dialect
	dataDir        string
	protocolEngine nodeProtocolEngine
	geoIP          *geoipinfra.Service
	maintenance    *maintenanceRunner
	requestLogs    *requestLogWriter
	adminLogins    *adminLoginLimiter
	ctx            context.Context
	cancel         context.CancelFunc
	closeOnce      sync.Once
}

type nodeProtocolEngine interface {
	DialNode(node nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error)
	DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error)
}

type closeableNodeProtocolEngine interface {
	nodeProtocolEngine
	Close() error
	InvalidateFingerprint(fingerprint string)
}

type geoIPCountryService interface {
	LookupCountry(ip string) string
	UpdateFromMetaCubeXLatest() error
	Close()
}

type dialTimeouts struct {
	ConnectTimeout time.Duration
	Deadline       time.Time
}

type nodeRecord struct {
	ID           string
	Name         string
	Type         string
	Server       string
	ServerPort   int
	Username     string
	Password     string
	RawJSON      string
	OutboundJSON string
	Enabled      bool
}

type proxyCredentialRecord struct {
	ID        string
	Remark    string
	ProfileID string
}

type selectedProxyPath struct {
	Credential        proxyCredentialRecord
	ProfileID         string
	Profile           string
	ProfileIdentifier string
	Node              nodeRecord
	FrontNode         nodeRecord
	ExitNode          nodeRecord
}

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

type profileEvaluationTarget struct {
	ID                           string
	Type                         string
	FixedNodeID                  string
	ExitNodeIDs                  []string
	ChainEvaluationMode          string
	TestURL                      string
	Filter                       candidateFilter
	CandidateLimit               int
	MinEvaluationIntervalSeconds int
	RelativeImprovementThreshold float64
	AbsoluteImprovementMS        int
	LastEvaluatedAt              int64
	ConfigVersion                int64
	ForceSwitch                  bool
	AutoEvaluationEnabled        bool
	NodeStickyEnabled            bool
}

type parsedSubscriptionNode struct {
	Name          string
	Type          string
	Server        string
	ServerPort    int
	Method        string
	UUID          string
	Flow          string
	Security      string
	AlterID       int
	TLSJSON       json.RawMessage
	TransportJSON json.RawMessage
	Username      string
	Password      string
	RawJSON       string
	OutboundJSON  string
}

type candidateFilter struct {
	EgressCountry     string
	EgressCountries   []string
	EgressCountryMode string
	NodeSourceMode    string
	SourceIDs         []string
	Protocols         []string
	NameIncludeRegex  string
	NameExcludeRegex  string
	ManualOnly        bool
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

type singleConnListener struct {
	conn net.Conn
	done bool
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.done {
		return nil, net.ErrClosed
	}
	l.done = true
	return l.conn, nil
}

func (l *singleConnListener) Close() error {
	if l.conn != nil && !l.done {
		return l.conn.Close()
	}
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}
