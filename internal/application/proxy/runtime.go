package proxy

import (
	"net"
	"time"
)

type Node struct {
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

type DialTimeouts struct {
	ConnectTimeout time.Duration
	Deadline       time.Time
}

type DialMetrics struct {
	CacheWaitMS    int64
	CacheBuildMS   int64
	OutboundDialMS int64
}

type DialResult struct {
	Conn    net.Conn
	Metrics DialMetrics
}

type ServiceOutboundPathKind string

const (
	ServiceOutboundPathSingle ServiceOutboundPathKind = "single"
	ServiceOutboundPathChain  ServiceOutboundPathKind = "chain"
)

type ServiceOutboundPath struct {
	Kind      ServiceOutboundPathKind
	Node      Node
	FrontNode Node
	ExitNode  Node
}

func SingleServiceOutboundPath(node Node) ServiceOutboundPath {
	return ServiceOutboundPath{Kind: ServiceOutboundPathSingle, Node: node}
}

func ChainServiceOutboundPath(frontNode, exitNode Node) ServiceOutboundPath {
	return ServiceOutboundPath{Kind: ServiceOutboundPathChain, FrontNode: frontNode, ExitNode: exitNode}
}

func (path ServiceOutboundPath) IsChain() bool {
	return path.Kind == ServiceOutboundPathChain || (path.FrontNode.ID != "" && path.ExitNode.ID != "")
}

type ServiceOutboundCacheLimits struct {
	Single int
	Chain  int
}

type ServiceOutboundCacheStats struct {
	ServiceSingle         int
	ServiceChain          int
	ServiceBuilding       int
	ActiveTemporaryCaches int
	TemporarySingle       int
	TemporaryChain        int
	TemporaryBuilding     int
}

type NodeProtocolEngine interface {
	DialNode(node Node, target string, timeouts DialTimeouts) (DialResult, error)
	DialChain(frontNode, exitNode Node, target string, timeouts DialTimeouts) (DialResult, error)
}

type TemporaryNodeProtocolEngine interface {
	NodeProtocolEngine
	Close() error
}

type TemporaryNodeProtocolEngineProvider interface {
	NewTemporaryNodeProtocolEngine() (TemporaryNodeProtocolEngine, error)
}

type ServiceOutboundCacheController interface {
	SetServiceOutboundSyncGeneration(generation uint64)
	SyncServiceOutboundCache(generation uint64, paths []ServiceOutboundPath) error
	WarmServiceOutboundPaths(paths []ServiceOutboundPath) error
	ServiceOutboundCacheLimits() ServiceOutboundCacheLimits
}

type CloseableNodeProtocolEngine interface {
	NodeProtocolEngine
	Close() error
	InvalidateFingerprint(fingerprint string)
}
