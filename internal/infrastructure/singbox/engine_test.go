package singbox

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
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
