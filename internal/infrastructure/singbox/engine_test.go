package singbox

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appproxy "proxygateway/internal/application/proxy"

	M "github.com/sagernet/sing/common/metadata"
)

func TestDialContextForTimeoutsUsesConnectTimeoutWhenEarlierThanProbeDeadline(t *testing.T) {
	startedAt := time.Now()
	probeDeadline := startedAt.Add(10 * time.Second)
	ctx, cancel := dialContextForTimeouts(appproxy.DialTimeouts{
		ConnectTimeout: 1500 * time.Millisecond,
		Deadline:       probeDeadline,
	})
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("context deadline is missing")
	}
	if !deadline.Before(probeDeadline) {
		t.Fatalf("deadline = %v, want before probe deadline %v", deadline, probeDeadline)
	}
	elapsedBudget := deadline.Sub(startedAt)
	if elapsedBudget < 1400*time.Millisecond || elapsedBudget > 1600*time.Millisecond {
		t.Fatalf("deadline budget = %s, want near connect timeout", elapsedBudget)
	}
}

func TestDialContextForTimeoutsUsesProbeDeadlineWhenEarlierThanConnectTimeout(t *testing.T) {
	probeDeadline := time.Now().Add(1500 * time.Millisecond)
	ctx, cancel := dialContextForTimeouts(appproxy.DialTimeouts{
		ConnectTimeout: 10 * time.Second,
		Deadline:       probeDeadline,
	})
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("context deadline is missing")
	}
	if !deadline.Equal(probeDeadline) {
		t.Fatalf("deadline = %v, want probe deadline %v", deadline, probeDeadline)
	}
}

func TestDialContextForTimeoutsKeepsConnectTimeoutForProxyDial(t *testing.T) {
	startedAt := time.Now()
	ctx, cancel := dialContextForTimeouts(appproxy.DialTimeouts{
		ConnectTimeout: 1500 * time.Millisecond,
	})
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("context deadline is missing")
	}
	elapsedBudget := deadline.Sub(startedAt)
	if elapsedBudget < 1400*time.Millisecond || elapsedBudget > 1600*time.Millisecond {
		t.Fatalf("deadline budget = %s, want near connect timeout", elapsedBudget)
	}
}

func TestDialCachedOutboundHardTimeoutReturnsBeforeOutboundDial(t *testing.T) {
	cache := newSingBoxOutboundCache()
	engine := &singBoxNodeProtocolEngine{cache: cache}
	conn := newCloseRecordingConn()
	outbound := &blockingOutbound{
		started: make(chan struct{}),
		release: make(chan struct{}),
		conn:    conn,
	}
	entry := newSingBoxCachedOutbound("slow", []string{"slow"}, outbound, nil)
	entry.refs = 1
	deadline := time.Now().Add(30 * time.Millisecond)

	startedAt := time.Now()
	result, err := engine.dialCachedOutbound(entry, M.Socksaddr{}, appproxy.DialTimeouts{
		Deadline: deadline,
	}, appproxy.DialMetrics{CacheBuildMS: 7})
	elapsed := time.Since(startedAt)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("dial error = %v, want context deadline exceeded", err)
	}
	if result.Conn != nil {
		t.Fatalf("dial conn = %#v, want nil on hard timeout", result.Conn)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("dial elapsed = %s, want hard timeout before blocked outbound returns", elapsed)
	}
	if result.Metrics.CacheBuildMS != 7 {
		t.Fatalf("cache build metric = %d, want preserved metric", result.Metrics.CacheBuildMS)
	}
	if result.Metrics.OutboundDialMS <= 0 || result.Metrics.OutboundDialMS > 250 {
		t.Fatalf("outbound dial metric = %d, want hard timeout duration", result.Metrics.OutboundDialMS)
	}
	if refs := cacheEntryRefs(cache, entry); refs != 1 {
		t.Fatalf("entry refs before blocked dial returns = %d, want 1", refs)
	}

	close(outbound.release)
	waitForConnClosed(t, conn)
	waitForCacheEntryRefs(t, cache, entry, 0)
}

func TestServiceOutboundCacheSyncIgnoresStaleGeneration(t *testing.T) {
	t.Parallel()

	cache := newSingBoxServiceOutboundCache()
	var closes int32
	entry := newSingBoxCachedOutbound("new-path", []string{"new-path"}, nil, func() error {
		atomic.AddInt32(&closes, 1)
		return nil
	})
	entry.namespace = singBoxOutboundCacheSingle
	cache.single[entry.key] = entry
	engine := &singBoxNodeProtocolEngine{cache: cache}

	engine.SetServiceOutboundSyncGeneration(2)
	if err := engine.SyncServiceOutboundCache(1, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.single["new-path"]; !ok {
		t.Fatal("stale sync removed newer service cache entry")
	}
	if got := atomic.LoadInt32(&closes); got != 0 {
		t.Fatalf("closes after stale sync = %d, want 0", got)
	}

	if err := engine.SyncServiceOutboundCache(2, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.single["new-path"]; ok {
		t.Fatal("current sync kept disallowed service cache entry")
	}
	if got := atomic.LoadInt32(&closes); got != 1 {
		t.Fatalf("closes after current sync = %d, want 1", got)
	}
}

func TestServiceOutboundDirectExitMapsToFrontSingleKey(t *testing.T) {
	t.Parallel()

	front := appproxy.Node{ID: "front", OutboundJSON: `{"type":"direct"}`}
	exit := appproxy.Node{ID: "exit", Type: "direct", OutboundJSON: `{"type":"direct"}`}
	_, frontFingerprint, err := runtimeOutboundJSONForNode(front)
	if err != nil {
		t.Fatal(err)
	}

	keys, err := serviceOutboundAllowedKeys([]appproxy.ServiceOutboundPath{
		appproxy.ChainServiceOutboundPath(front, exit),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := keys.single[frontFingerprint]; !ok {
		t.Fatalf("single keys = %#v, want front fingerprint %q", keys.single, frontFingerprint)
	}
	if len(keys.chain) != 0 {
		t.Fatalf("chain keys = %#v, want no chain key for direct exit", keys.chain)
	}
}

func TestSingBoxEngineInvalidatesTemporaryCaches(t *testing.T) {
	t.Parallel()

	engine := &singBoxNodeProtocolEngine{
		cache:           newSingBoxServiceOutboundCache(),
		temporaryCaches: make(map[*singBoxOutboundCache]struct{}),
	}
	tempEngine, err := engine.NewTemporaryNodeProtocolEngine()
	if err != nil {
		t.Fatal(err)
	}
	temp := tempEngine.(*singBoxTemporaryNodeProtocolEngine)
	var serviceCloses int32
	var tempCloses int32
	serviceEntry := newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, func() error {
		atomic.AddInt32(&serviceCloses, 1)
		return nil
	})
	serviceEntry.namespace = singBoxOutboundCacheSingle
	tempEntry := newSingBoxCachedOutbound("fingerprint-1", []string{"fingerprint-1"}, nil, func() error {
		atomic.AddInt32(&tempCloses, 1)
		return nil
	})
	tempEntry.namespace = singBoxOutboundCacheSingle
	engine.cache.single["fingerprint-1"] = serviceEntry
	temp.cache.single["fingerprint-1"] = tempEntry

	engine.InvalidateFingerprint("fingerprint-1")
	if got := atomic.LoadInt32(&serviceCloses); got != 1 {
		t.Fatalf("service closes = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&tempCloses); got != 1 {
		t.Fatalf("temporary closes = %d, want 1", got)
	}
	if err := tempEngine.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.NewTemporaryNodeProtocolEngine(); err != nil {
		t.Fatalf("new temporary before service close = %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.NewTemporaryNodeProtocolEngine(); !errors.Is(err, errSingBoxOutboundCacheClosed) {
		t.Fatalf("new temporary after service close = %v, want cache closed", err)
	}
}

type blockingOutbound struct {
	startOnce sync.Once
	started   chan struct{}
	release   chan struct{}
	conn      net.Conn
	err       error
}

func (o *blockingOutbound) Type() string {
	return "test"
}

func (o *blockingOutbound) Tag() string {
	return "test"
}

func (o *blockingOutbound) Network() []string {
	return []string{"tcp"}
}

func (o *blockingOutbound) Dependencies() []string {
	return nil
}

func (o *blockingOutbound) DialContext(context.Context, string, M.Socksaddr) (net.Conn, error) {
	o.startOnce.Do(func() { close(o.started) })
	<-o.release
	return o.conn, o.err
}

func (o *blockingOutbound) ListenPacket(context.Context, M.Socksaddr) (net.PacketConn, error) {
	return nil, errors.New("not implemented")
}

type closeRecordingConn struct {
	closeOnce sync.Once
	closed    chan struct{}
}

func newCloseRecordingConn() *closeRecordingConn {
	return &closeRecordingConn{closed: make(chan struct{})}
}

func (c *closeRecordingConn) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (c *closeRecordingConn) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func (c *closeRecordingConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *closeRecordingConn) LocalAddr() net.Addr {
	return nil
}

func (c *closeRecordingConn) RemoteAddr() net.Addr {
	return nil
}

func (c *closeRecordingConn) SetDeadline(time.Time) error {
	return nil
}

func (c *closeRecordingConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *closeRecordingConn) SetWriteDeadline(time.Time) error {
	return nil
}

func cacheEntryRefs(cache *singBoxOutboundCache, entry *singBoxCachedOutbound) int {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return entry.refs
}

func waitForCacheEntryRefs(t *testing.T, cache *singBoxOutboundCache, entry *singBoxCachedOutbound, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if refs := cacheEntryRefs(cache, entry); refs == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for entry refs = %d", want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func waitForConnClosed(t *testing.T, conn *closeRecordingConn) {
	t.Helper()
	select {
	case <-conn.closed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for late dial connection close")
	}
}
