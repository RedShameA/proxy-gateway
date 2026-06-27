package maintenance

import (
	"context"
	"sync"
	"time"
)

type RunnerCallbacks struct {
	Context    context.Context
	Interval   time.Duration
	OnStarted  func()
	OnStopped  func()
	OnWake     func()
	OnTick     func()
	EnqueueDue func()
	RunQueued  func()
}

type Runner struct {
	callbacks RunnerCallbacks
	started   bool
	startMu   sync.Mutex
	wake      chan struct{}
}

func NewRunner(callbacks RunnerCallbacks) *Runner {
	if callbacks.Interval <= 0 {
		callbacks.Interval = 30 * time.Second
	}
	return &Runner{
		callbacks: callbacks,
		wake:      make(chan struct{}, 1),
	}
}

func (r *Runner) Start() {
	if r == nil {
		return
	}
	r.startMu.Lock()
	defer r.startMu.Unlock()
	if r.started {
		return
	}
	r.started = true
	r.call(r.callbacks.OnStarted)
	go r.loop()
}

func (r *Runner) Notify() {
	if r == nil {
		return
	}
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runner) loop() {
	ticker := time.NewTicker(r.callbacks.Interval)
	defer ticker.Stop()
	r.enqueueAndRun()
	for {
		select {
		case <-r.context().Done():
			r.call(r.callbacks.OnStopped)
			return
		case <-r.wake:
			r.call(r.callbacks.OnWake)
			r.enqueueAndRun()
		case <-ticker.C:
			r.call(r.callbacks.OnTick)
			r.enqueueAndRun()
		}
	}
}

func (r *Runner) enqueueAndRun() {
	r.call(r.callbacks.EnqueueDue)
	r.call(r.callbacks.RunQueued)
}

func (r *Runner) context() context.Context {
	if r.callbacks.Context == nil {
		return context.Background()
	}
	return r.callbacks.Context
}

func (r *Runner) call(fn func()) {
	if fn != nil {
		fn()
	}
}
