package app

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	appobservations "proxygateway/internal/application/observations"
	appproxy "proxygateway/internal/application/proxy"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

var serviceOutboundTestPortSeq int32 = 18080

func TestServiceOutboundPlanRandomUsesUsableObservationAndFixedDoesNot(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zapcore.WarnLevel)
	g := NewForTest(t, WithLogger(zap.New(core)), WithoutMaintenanceRunner())
	if engine, ok := g.protocolEngine.(closeableNodeProtocolEngine); ok {
		_ = engine.Close()
	}
	g.protocolEngine = &serviceOutboundTestController{limits: appproxy.ServiceOutboundCacheLimits{Single: 1, Chain: 1}}

	usableA := createDirectNodeForServiceOutboundTest(t, g, "usable-a")
	usableB := createDirectNodeForServiceOutboundTest(t, g, "usable-b")
	unobserved := createDirectNodeForServiceOutboundTest(t, g, "unobserved")
	if err := g.nodeObservationRepo.SaveSuccess(usableA.ID, appobservations.SuccessRecord{EgressIP: "198.51.100.1", EgressCountry: "US", LatencyMS: 10}, unixMillisNow()); err != nil {
		t.Fatal(err)
	}
	if err := g.nodeObservationRepo.SaveSuccess(usableB.ID, appobservations.SuccessRecord{EgressIP: "198.51.100.2", EgressCountry: "US", LatencyMS: 11}, unixMillisNow()); err != nil {
		t.Fatal(err)
	}

	if _, err := g.createAccessProfile(accessProfilePatchRequest{
		Name: stringPtr("random"),
		Type: stringPtr("random"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.createAccessProfile(accessProfilePatchRequest{
		Name:        stringPtr("fixed"),
		Type:        stringPtr("fixed_node"),
		FixedNodeID: stringPtr(unobserved.ID),
	}); err != nil {
		t.Fatal(err)
	}

	plan, err := g.buildGlobalServiceOutboundPlan(g.ctx)
	if err != nil {
		t.Fatal(err)
	}
	allowed := serviceOutboundTestPathNodeIDs(plan.Allowed)
	for _, nodeID := range []string{usableA.ID, usableB.ID, unobserved.ID} {
		if !allowed[nodeID] {
			t.Fatalf("allowed node IDs = %#v, want %s", allowed, nodeID)
		}
	}
	if len(allowed) != 3 {
		t.Fatalf("allowed node IDs = %#v, want only two random usable nodes plus fixed unobserved node", allowed)
	}
	warm := serviceOutboundTestPathNodeIDs(plan.Warm)
	if !warm[usableA.ID] || !warm[usableB.ID] || warm[unobserved.ID] || len(warm) != 2 {
		t.Fatalf("warm node IDs = %#v, want only random usable nodes", warm)
	}

	var foundWarn bool
	for _, entry := range observed.All() {
		if entry.Message != "random profile service outbound candidates exceed hard cap" {
			continue
		}
		fields := entry.ContextMap()
		if fields["candidate_count"] == int64(2) && fields["hard_cap"] == int64(1) {
			foundWarn = true
			break
		}
	}
	if !foundWarn {
		t.Fatalf("random hard-cap warn not found in %#v", observed.All())
	}
}

func TestObserveNodeUsesTemporaryProbeClientAndCloses(t *testing.T) {
	t.Parallel()

	g := NewForTest(t, WithoutMaintenanceRunner())
	if engine, ok := g.protocolEngine.(closeableNodeProtocolEngine); ok {
		_ = engine.Close()
	}
	provider := &temporaryProbeProviderForTest{body: "ip=203.0.113.10\nloc=US\n"}
	g.protocolEngine = provider
	node := createDirectNodeForServiceOutboundTest(t, g, "observed")

	ok, err := g.observeNode(node, "http://example.test/trace", evaluationSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("observeNode returned false, want true")
	}
	if got := atomic.LoadInt32(&provider.created); got != 1 {
		t.Fatalf("temporary engines created = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&provider.closed); got != 1 {
		t.Fatalf("temporary engines closed = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&provider.serviceDials); got != 0 {
		t.Fatalf("service engine dials = %d, want 0", got)
	}
}

func createDirectNodeForServiceOutboundTest(t *testing.T, g *Gateway, name string) nodeRecord {
	t.Helper()
	port := int(atomic.AddInt32(&serviceOutboundTestPortSeq, 1))
	created, err := g.createNodeSource(nodeCreateInput{Name: name, Type: "http", Server: "127.0.0.1", ServerPort: port})
	if err != nil {
		t.Fatal(err)
	}
	nodeID, _ := created["id"].(string)
	node, err := g.loadNode(nodeID)
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func serviceOutboundTestPathNodeIDs(paths []appproxy.ServiceOutboundPath) map[string]bool {
	ids := map[string]bool{}
	for _, path := range paths {
		if path.IsChain() {
			ids[path.FrontNode.ID] = true
			ids[path.ExitNode.ID] = true
			continue
		}
		ids[path.Node.ID] = true
	}
	return ids
}

type serviceOutboundTestController struct {
	limits appproxy.ServiceOutboundCacheLimits
}

func (c *serviceOutboundTestController) DialNode(node nodeRecord, target string, timeouts dialTimeouts) (dialResult, error) {
	return dialResult{}, errors.New("unexpected service dial")
}

func (c *serviceOutboundTestController) DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (dialResult, error) {
	return dialResult{}, errors.New("unexpected service dial")
}

func (c *serviceOutboundTestController) SetServiceOutboundSyncGeneration(uint64) {}

func (c *serviceOutboundTestController) SyncServiceOutboundCache(uint64, []appproxy.ServiceOutboundPath) error {
	return nil
}

func (c *serviceOutboundTestController) WarmServiceOutboundPaths([]appproxy.ServiceOutboundPath) error {
	return nil
}

func (c *serviceOutboundTestController) ServiceOutboundCacheLimits() appproxy.ServiceOutboundCacheLimits {
	return c.limits
}

type temporaryProbeProviderForTest struct {
	body         string
	created      int32
	closed       int32
	serviceDials int32
}

func (p *temporaryProbeProviderForTest) DialNode(node nodeRecord, target string, timeouts dialTimeouts) (dialResult, error) {
	atomic.AddInt32(&p.serviceDials, 1)
	return dialResult{}, errors.New("service engine should not be used for observation probes")
}

func (p *temporaryProbeProviderForTest) DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (dialResult, error) {
	atomic.AddInt32(&p.serviceDials, 1)
	return dialResult{}, errors.New("service engine should not be used for observation probes")
}

func (p *temporaryProbeProviderForTest) NewTemporaryNodeProtocolEngine() (appproxy.TemporaryNodeProtocolEngine, error) {
	atomic.AddInt32(&p.created, 1)
	return &temporaryProbeEngineForTest{provider: p}, nil
}

type temporaryProbeEngineForTest struct {
	provider  *temporaryProbeProviderForTest
	closeOnce sync.Once
}

func (e *temporaryProbeEngineForTest) DialNode(node nodeRecord, target string, timeouts dialTimeouts) (dialResult, error) {
	return dialResult{Conn: probeHTTPConnForTest(e.provider.body)}, nil
}

func (e *temporaryProbeEngineForTest) DialChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (dialResult, error) {
	return e.DialNode(frontNode, target, timeouts)
}

func (e *temporaryProbeEngineForTest) Close() error {
	e.closeOnce.Do(func() {
		atomic.AddInt32(&e.provider.closed, 1)
	})
	return nil
}

func probeHTTPConnForTest(body string) net.Conn {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		_, _ = http.ReadRequest(bufio.NewReader(server))
		_, _ = server.Write([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(body), body)))
	}()
	return client
}
