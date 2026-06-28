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

type NodeProtocolEngine interface {
	DialNode(node Node, target string, timeouts DialTimeouts) (DialResult, error)
	DialChain(frontNode, exitNode Node, target string, timeouts DialTimeouts) (DialResult, error)
}

type CloseableNodeProtocolEngine interface {
	NodeProtocolEngine
	Close() error
	InvalidateFingerprint(fingerprint string)
}
