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

type nilConnEngine struct{}

func (nilConnEngine) DialNode(appproxy.Node, string, appproxy.DialTimeouts) (net.Conn, error) {
	return nil, nil
}

func (nilConnEngine) DialChain(appproxy.Node, appproxy.Node, string, appproxy.DialTimeouts) (net.Conn, error) {
	return nil, nil
}

type typedNilConnEngine struct{}

func (typedNilConnEngine) DialNode(appproxy.Node, string, appproxy.DialTimeouts) (net.Conn, error) {
	var conn *typedNilConn
	return conn, nil
}

func (typedNilConnEngine) DialChain(appproxy.Node, appproxy.Node, string, appproxy.DialTimeouts) (net.Conn, error) {
	var conn *typedNilConn
	return conn, nil
}

type closedPipeEngine struct{}

func (closedPipeEngine) DialNode(appproxy.Node, string, appproxy.DialTimeouts) (net.Conn, error) {
	clientConn, serverConn := net.Pipe()
	_ = serverConn.Close()
	return clientConn, nil
}

func (closedPipeEngine) DialChain(appproxy.Node, appproxy.Node, string, appproxy.DialTimeouts) (net.Conn, error) {
	return closedPipeEngine{}.DialNode(appproxy.Node{}, "", appproxy.DialTimeouts{})
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
