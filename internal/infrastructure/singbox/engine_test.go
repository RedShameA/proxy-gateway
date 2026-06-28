package singbox

import (
	"testing"
	"time"

	appproxy "proxygateway/internal/application/proxy"
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
