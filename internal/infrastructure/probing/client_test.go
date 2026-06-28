package probing

import (
	"errors"
	"net"
	"testing"
	"time"

	appproxy "proxygateway/internal/application/proxy"
)

func TestClientReturnsErrorWhenEngineReturnsNilConnection(t *testing.T) {
	for _, tt := range []struct {
		name   string
		engine appproxy.NodeProtocolEngine
	}{
		{name: "nil interface", engine: nilConnEngine{}},
		{name: "typed nil", engine: typedNilConnEngine{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{Engine: tt.engine}
			node := appproxy.Node{ID: "node_1", Name: "node 1"}

			if _, err := client.FetchThroughNode(node, "https://example.com/generate_204", appproxy.DialTimeouts{}); !errors.Is(err, errNilDialConn) {
				t.Fatalf("FetchThroughNode error = %v, want errNilDialConn", err)
			}
			if _, err := client.FetchThroughChain(node, node, "https://example.com/generate_204", appproxy.DialTimeouts{}); !errors.Is(err, errNilDialConn) {
				t.Fatalf("FetchThroughChain error = %v, want errNilDialConn", err)
			}
			if _, err := client.ProbeChainLink(node, node, "https://example.com/generate_204", appproxy.DialTimeouts{}); !errors.Is(err, errNilDialConn) {
				t.Fatalf("ProbeChainLink error = %v, want errNilDialConn", err)
			}
		})
	}
}

func TestFetchThroughNodeReturnsHandshakeErrorWithoutDeadlinePanic(t *testing.T) {
	client := Client{Engine: closedPipeEngine{}}
	node := appproxy.Node{ID: "node_1", Name: "node 1"}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("FetchThroughNode panicked: %v", recovered)
		}
	}()
	_, err := client.FetchThroughNode(node, "https://example.com/generate_204", appproxy.DialTimeouts{
		Deadline: time.Now().Add(time.Second),
	})
	if err == nil {
		t.Fatal("FetchThroughNode error = nil, want TLS handshake error")
	}
}

func TestClientReturnsElapsedDurationOnDialError(t *testing.T) {
	client := Client{Engine: delayedFailingEngine{delay: 20 * time.Millisecond}}
	node := appproxy.Node{ID: "node_1", Name: "node 1"}

	nodeResult, err := client.FetchThroughNode(node, "https://example.com/generate_204", appproxy.DialTimeouts{})
	if err == nil {
		t.Fatal("FetchThroughNode error = nil, want dial error")
	}
	if nodeResult.DurationMS <= 0 {
		t.Fatalf("FetchThroughNode duration = %d, want positive", nodeResult.DurationMS)
	}
	if nodeResult.DialDurationMS <= 0 {
		t.Fatalf("FetchThroughNode dial duration = %d, want positive", nodeResult.DialDurationMS)
	}
	if nodeResult.HTTPDurationMS != 0 {
		t.Fatalf("FetchThroughNode HTTP duration = %d, want 0 on dial error", nodeResult.HTTPDurationMS)
	}

	chainResult, err := client.FetchThroughChain(node, node, "https://example.com/generate_204", appproxy.DialTimeouts{})
	if err == nil {
		t.Fatal("FetchThroughChain error = nil, want dial error")
	}
	if chainResult.DurationMS <= 0 {
		t.Fatalf("FetchThroughChain duration = %d, want positive", chainResult.DurationMS)
	}
	if chainResult.DialDurationMS <= 0 {
		t.Fatalf("FetchThroughChain dial duration = %d, want positive", chainResult.DialDurationMS)
	}
	if chainResult.HTTPDurationMS != 0 {
		t.Fatalf("FetchThroughChain HTTP duration = %d, want 0 on dial error", chainResult.HTTPDurationMS)
	}

	chainLinkResult, err := client.ProbeChainLink(node, node, "https://example.com/generate_204", appproxy.DialTimeouts{})
	if err == nil {
		t.Fatal("ProbeChainLink error = nil, want dial error")
	}
	if chainLinkResult.DurationMS <= 0 {
		t.Fatalf("ProbeChainLink duration = %d, want positive", chainLinkResult.DurationMS)
	}
	if chainLinkResult.DialDurationMS <= 0 {
		t.Fatalf("ProbeChainLink dial duration = %d, want positive", chainLinkResult.DialDurationMS)
	}
}

func TestClientPreservesDialMetricsOnDialError(t *testing.T) {
	metrics := appproxy.DialMetrics{
		CacheWaitMS:    11,
		CacheBuildMS:   22,
		OutboundDialMS: 33,
	}
	client := Client{Engine: metricsFailingEngine{metrics: metrics}}
	node := appproxy.Node{ID: "node_1", Name: "node 1"}

	result, err := client.FetchThroughChain(node, node, "https://example.com/generate_204", appproxy.DialTimeouts{})
	if err == nil {
		t.Fatal("FetchThroughChain error = nil, want dial error")
	}
	if result.DialMetrics != metrics {
		t.Fatalf("dial metrics = %#v, want %#v", result.DialMetrics, metrics)
	}
}

type nilConnEngine struct{}

func (nilConnEngine) DialNode(appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	return appproxy.DialResult{}, nil
}

func (nilConnEngine) DialChain(appproxy.Node, appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	return appproxy.DialResult{}, nil
}

type typedNilConnEngine struct{}

func (typedNilConnEngine) DialNode(appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	var conn *typedNilConn
	return appproxy.DialResult{Conn: conn}, nil
}

func (typedNilConnEngine) DialChain(appproxy.Node, appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	var conn *typedNilConn
	return appproxy.DialResult{Conn: conn}, nil
}

type closedPipeEngine struct{}

func (closedPipeEngine) DialNode(appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	clientConn, serverConn := net.Pipe()
	_ = serverConn.Close()
	return appproxy.DialResult{Conn: clientConn}, nil
}

func (closedPipeEngine) DialChain(appproxy.Node, appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	return closedPipeEngine{}.DialNode(appproxy.Node{}, "", appproxy.DialTimeouts{})
}

type delayedFailingEngine struct {
	delay time.Duration
}

func (e delayedFailingEngine) DialNode(appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	time.Sleep(e.delay)
	return appproxy.DialResult{}, errors.New("delayed dial failed")
}

func (e delayedFailingEngine) DialChain(appproxy.Node, appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	return e.DialNode(appproxy.Node{}, "", appproxy.DialTimeouts{})
}

type metricsFailingEngine struct {
	metrics appproxy.DialMetrics
}

func (e metricsFailingEngine) DialNode(appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	return appproxy.DialResult{Metrics: e.metrics}, errors.New("metrics dial failed")
}

func (e metricsFailingEngine) DialChain(appproxy.Node, appproxy.Node, string, appproxy.DialTimeouts) (appproxy.DialResult, error) {
	return e.DialNode(appproxy.Node{}, "", appproxy.DialTimeouts{})
}

type typedNilConn struct{}

func (*typedNilConn) Read([]byte) (int, error) {
	return 0, nil
}

func (*typedNilConn) Write([]byte) (int, error) {
	return 0, nil
}

func (*typedNilConn) Close() error {
	return nil
}

func (*typedNilConn) LocalAddr() net.Addr {
	return nil
}

func (*typedNilConn) RemoteAddr() net.Addr {
	return nil
}

func (*typedNilConn) SetDeadline(time.Time) error {
	return nil
}

func (*typedNilConn) SetReadDeadline(time.Time) error {
	return nil
}

func (*typedNilConn) SetWriteDeadline(time.Time) error {
	return nil
}
