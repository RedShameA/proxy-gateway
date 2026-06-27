package maintenance

import (
	"context"
	"testing"
	"time"
)

func TestRunnerStartsRunsWakesAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	ran := make(chan struct{}, 2)
	woke := make(chan struct{}, 1)
	stopped := make(chan struct{}, 1)
	runner := NewRunner(RunnerCallbacks{
		Context:  ctx,
		Interval: time.Hour,
		OnStarted: func() {
			started <- struct{}{}
		},
		OnWake: func() {
			woke <- struct{}{}
		},
		OnStopped: func() {
			stopped <- struct{}{}
		},
		EnqueueDue: func() {
			ran <- struct{}{}
		},
		RunQueued: func() {},
	})
	runner.Start()
	waitForRunnerSignal(t, started, "started")
	waitForRunnerSignal(t, ran, "initial run")
	runner.Notify()
	waitForRunnerSignal(t, woke, "wake")
	waitForRunnerSignal(t, ran, "wake run")
	cancel()
	waitForRunnerSignal(t, stopped, "stopped")
}

func waitForRunnerSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("runner did not signal %s", name)
	}
}
