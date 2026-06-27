package app

import (
	"net"
	"net/http"
	"net/http/httptest"
	probinginfra "proxygateway/internal/infrastructure/probing"
	proxyiface "proxygateway/internal/interfaces/proxy"
	"testing"
	"time"
)

func TestObserveNodeAppliesProbeDeadlineAfterDial(t *testing.T) {
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte("ip=203.0.113.10\nloc=US\n"))
	}))
	t.Cleanup(probe.Close)

	g := NewForTest(t)
	start := time.Now()
	ok, err := g.observeNode(nodeRecord{
		ID:      "node_timeout",
		Name:    "timeout direct",
		Type:    "direct",
		Enabled: true,
	}, probe.URL, evaluationSettings{
		ConnectTimeoutSeconds: 1,
		ProbeTimeoutSeconds:   1,
	})
	elapsed := time.Since(start)
	if ok {
		t.Fatal("observeNode unexpectedly succeeded")
	}
	if err == nil {
		t.Fatal("observeNode error is nil")
	}
	if elapsed >= 1800*time.Millisecond {
		t.Fatalf("observeNode elapsed = %s, want probe deadline near 1s", elapsed)
	}
}

func TestFetchTestURLThroughChainAppliesProbeDeadlineAfterDial(t *testing.T) {
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(probe.Close)

	g := NewForTest(t)
	g.protocolEngine = directTestEngine{}
	start := time.Now()
	_, _, err := g.fetchTestURLThroughChain(
		nodeRecord{ID: "front", Name: "front", Type: "direct", Enabled: true},
		nodeRecord{ID: "exit", Name: "exit", Type: "direct", Enabled: true},
		probe.URL,
		evaluationSettings{ConnectTimeoutSeconds: 1, ProbeTimeoutSeconds: 1},
	)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("fetchTestURLThroughChain error is nil")
	}
	if elapsed >= 1800*time.Millisecond {
		t.Fatalf("fetchTestURLThroughChain elapsed = %s, want probe deadline near 1s", elapsed)
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
