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

type NodeProtocolEngine interface {
	DialNode(node Node, target string, timeouts DialTimeouts) (net.Conn, error)
	DialChain(frontNode, exitNode Node, target string, timeouts DialTimeouts) (net.Conn, error)
}

type CloseableNodeProtocolEngine interface {
	NodeProtocolEngine
	Close() error
	InvalidateFingerprint(fingerprint string)
}
