package app

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	probinginfra "proxygateway/internal/infrastructure/probing"
	proxyiface "proxygateway/internal/interfaces/proxy"
	"sync"
	"testing"
	"time"
)

func TestObserveNodeAppliesProbeDeadlineAfterDial(t *testing.T) {
	engine := newDeadlineRecordingEngine("ip=203.0.113.10\nloc=US\n")
	g := NewForTest(t)
	g.protocolEngine = engine
	ok, err := g.observeNode(nodeRecord{
		ID:      "node_timeout",
		Name:    "timeout direct",
		Type:    "direct",
		Enabled: true,
	}, "http://probe.example/generate_204", evaluationSettings{
		ConnectTimeoutSeconds: 1,
		ProbeTimeoutSeconds:   1,
	})
	if !ok || err != nil {
		t.Fatalf("observeNode = %v, %v; want success", ok, err)
	}
	dialDeadline, connDeadline := engine.nodeDeadlines()
	if dialDeadline.IsZero() {
		t.Fatal("DialNode timeout deadline is zero")
	}
	if connDeadline.IsZero() {
		t.Fatal("connection deadline was not set after DialNode")
	}
	if !connDeadline.Equal(dialDeadline) {
		t.Fatalf("connection deadline = %v, want dial timeout deadline %v", connDeadline, dialDeadline)
	}
}

func TestFetchTestURLThroughChainAppliesProbeDeadlineAfterDial(t *testing.T) {
	engine := newDeadlineRecordingEngine("ok")
	g := NewForTest(t)
	g.protocolEngine = engine
	_, _, err := g.fetchTestURLThroughChain(
		nodeRecord{ID: "front", Name: "front", Type: "direct", Enabled: true},
		nodeRecord{ID: "exit", Name: "exit", Type: "direct", Enabled: true},
		"http://probe.example/generate_204",
		evaluationSettings{ConnectTimeoutSeconds: 1, ProbeTimeoutSeconds: 1},
	)
	if err != nil {
		t.Fatalf("fetchTestURLThroughChain error = %v, want nil", err)
	}
	dialDeadline, connDeadline := engine.chainDeadlines()
	if dialDeadline.IsZero() {
		t.Fatal("DialChain timeout deadline is zero")
	}
	if connDeadline.IsZero() {
		t.Fatal("connection deadline was not set after DialChain")
	}
	if !connDeadline.Equal(dialDeadline) {
		t.Fatalf("connection deadline = %v, want dial timeout deadline %v", connDeadline, dialDeadline)
	}
}

func TestProxyTargetHelpersAddDefaultPortForIPv6(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://[2001:db8::1]/demo", nil)
	if got, want := proxyiface.AbsoluteProxyURLTargetHost(req), "[2001:db8::1]:80"; got != want {
		t.Fatalf("absoluteProxyURLTargetHost = %q, want %q", got, want)
	}

	outbound, err := probinginfra.BuildOutboundGETRequest("https://[2001:db8::2]/probe")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := outbound.TargetHost, "[2001:db8::2]:443"; got != want {
		t.Fatalf("outbound target = %q, want %q", got, want)
	}
}

type directTestEngine struct{}

func (directTestEngine) DialNode(node nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	return dialTestTarget(target, timeouts)
}

func (directTestEngine) DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	return dialTestTarget(target, timeouts)
}

func dialTestTarget(target string, timeouts dialTimeouts) (net.Conn, error) {
	timeout := timeouts.ConnectTimeout
	if timeout <= 0 {
		timeout = time.Second
	}
	return net.DialTimeout("tcp", target, timeout)
}

type deadlineRecordingEngine struct {
	mu                 sync.Mutex
	responseBody       string
	nodeDialDeadline   time.Time
	nodeConnDeadlines  []time.Time
	chainDialDeadline  time.Time
	chainConnDeadlines []time.Time
}

func newDeadlineRecordingEngine(responseBody string) *deadlineRecordingEngine {
	return &deadlineRecordingEngine{responseBody: responseBody}
}

func (e *deadlineRecordingEngine) DialNode(node nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	e.mu.Lock()
	e.nodeDialDeadline = timeouts.Deadline
	e.mu.Unlock()
	return e.newConn(func(deadline time.Time) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.nodeConnDeadlines = append(e.nodeConnDeadlines, deadline)
	}), nil
}

func (e *deadlineRecordingEngine) DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	e.mu.Lock()
	e.chainDialDeadline = timeouts.Deadline
	e.mu.Unlock()
	return e.newConn(func(deadline time.Time) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.chainConnDeadlines = append(e.chainConnDeadlines, deadline)
	}), nil
}

func (e *deadlineRecordingEngine) nodeDeadlines() (time.Time, time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nodeDialDeadline, firstNonZeroDeadline(e.nodeConnDeadlines)
}

func (e *deadlineRecordingEngine) chainDeadlines() (time.Time, time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.chainDialDeadline, firstNonZeroDeadline(e.chainConnDeadlines)
}

func (e *deadlineRecordingEngine) newConn(recordDeadline func(time.Time)) net.Conn {
	clientConn, serverConn := net.Pipe()
	go serveProbeResponse(serverConn, e.responseBody)
	return deadlineRecordingConn{Conn: clientConn, recordDeadline: recordDeadline}
}

type deadlineRecordingConn struct {
	net.Conn
	recordDeadline func(time.Time)
}

func (c deadlineRecordingConn) SetDeadline(deadline time.Time) error {
	c.recordDeadline(deadline)
	return c.Conn.SetDeadline(deadline)
}

func firstNonZeroDeadline(deadlines []time.Time) time.Time {
	for _, deadline := range deadlines {
		if !deadline.IsZero() {
			return deadline
		}
	}
	return time.Time{}
}

func serveProbeResponse(conn net.Conn, body string) {
	defer conn.Close()
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err == nil && req.Body != nil {
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
}
