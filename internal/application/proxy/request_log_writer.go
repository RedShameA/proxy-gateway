package proxy

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

const (
	RequestLogFlushTimeout = 2 * time.Second

	requestLogQueueCapacity = 4096
	requestLogDropWarnEvery = 100
)

type requestLogEventKind int

const (
	requestLogEventStart requestLogEventKind = iota + 1
	requestLogEventFinish
	requestLogEventFailure
)

type requestLogEvent struct {
	kind         requestLogEventKind
	startRecord  RequestLogStartRecord
	finishRecord RequestLogFinishRecord
	failure      RequestLogFailureRecord
}

type RequestLogWriter struct {
	repo    RequestLogRepository
	events  chan requestLogEvent
	stop    chan struct{}
	done    chan struct{}
	mu      sync.Mutex
	closing bool
	dropped atomic.Int64
	logger  *zap.Logger
}

func NewRequestLogWriter(repo RequestLogRepository, logger *zap.Logger) *RequestLogWriter {
	w := &RequestLogWriter{
		repo:   repo,
		events: make(chan requestLogEvent, requestLogQueueCapacity),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		logger: ensureRequestLogLogger(logger),
	}
	go w.run()
	return w
}

func (w *RequestLogWriter) EnqueueStart(record RequestLogStartRecord) bool {
	return w.enqueue(requestLogEvent{kind: requestLogEventStart, startRecord: record})
}

func (w *RequestLogWriter) EnqueueFinish(record RequestLogFinishRecord) bool {
	return w.enqueue(requestLogEvent{kind: requestLogEventFinish, finishRecord: record})
}

func (w *RequestLogWriter) EnqueueFailure(record RequestLogFailureRecord) bool {
	return w.enqueue(requestLogEvent{kind: requestLogEventFailure, failure: record})
}

func (w *RequestLogWriter) enqueue(event requestLogEvent) bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing {
		w.recordDrop(event, "closing")
		return false
	}
	select {
	case w.events <- event:
		return true
	default:
		w.recordDrop(event, "queue_full")
		return false
	}
}

func (w *RequestLogWriter) recordDrop(event requestLogEvent, reason string) {
	dropped := w.dropped.Add(1)
	if dropped == 1 || dropped%requestLogDropWarnEvery == 0 {
		record := event.logFields()
		ensureRequestLogLogger(w.logger).Warn("request log event dropped",
			zap.String("reason", reason),
			zap.Int64("dropped_total", dropped),
			zap.String("event_kind", requestLogEventKindName(event.kind)),
			zap.String("log_id", record.id),
			zap.String("target_host", record.targetHost),
			zap.String("failure_stage", record.failureStage),
		)
	}
}

func (w *RequestLogWriter) DroppedCount() int64 {
	if w == nil {
		return 0
	}
	return w.dropped.Load()
}

func (w *RequestLogWriter) Close(timeout time.Duration) bool {
	if w == nil {
		return true
	}
	w.mu.Lock()
	if !w.closing {
		w.closing = true
		close(w.stop)
	}
	w.mu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-w.done:
		return true
	case <-timer.C:
		return false
	}
}

func (w *RequestLogWriter) run() {
	defer close(w.done)
	for {
		select {
		case event := <-w.events:
			w.write(event)
		case <-w.stop:
			for {
				select {
				case event := <-w.events:
					w.write(event)
				default:
					return
				}
			}
		}
	}
}

func (w *RequestLogWriter) write(event requestLogEvent) {
	var err error
	switch event.kind {
	case requestLogEventStart:
		err = w.repo.InsertStart(context.Background(), event.startRecord)
	case requestLogEventFinish:
		err = w.repo.Finish(context.Background(), event.finishRecord)
	case requestLogEventFailure:
		err = w.repo.InsertFailure(context.Background(), event.failure)
	default:
		ensureRequestLogLogger(w.logger).Warn("unknown request log event kind",
			zap.Int("event_kind", int(event.kind)),
			zap.String("log_id", event.logFields().id),
		)
		return
	}
	if err != nil {
		record := event.logFields()
		ensureRequestLogLogger(w.logger).Error("write request log event failed",
			zap.String("event_kind", requestLogEventKindName(event.kind)),
			zap.String("log_id", record.id),
			zap.String("target_host", record.targetHost),
			zap.String("failure_stage", record.failureStage),
			zap.Error(err),
		)
	}
}

type requestLogEventFields struct {
	id           string
	targetHost   string
	failureStage string
}

func (event requestLogEvent) logFields() requestLogEventFields {
	switch event.kind {
	case requestLogEventStart:
		return requestLogEventFields{id: event.startRecord.ID, targetHost: event.startRecord.TargetHost}
	case requestLogEventFinish:
		return requestLogEventFields{id: event.finishRecord.ID, failureStage: event.finishRecord.FailureStage}
	case requestLogEventFailure:
		return requestLogEventFields{id: event.failure.ID, targetHost: event.failure.TargetHost, failureStage: event.failure.FailureStage}
	default:
		return requestLogEventFields{}
	}
}

func requestLogEventKindName(kind requestLogEventKind) string {
	switch kind {
	case requestLogEventStart:
		return RequestLogEventKindStart
	case requestLogEventFinish:
		return RequestLogEventKindFinish
	case requestLogEventFailure:
		return RequestLogEventKindFailure
	default:
		return RequestLogEventKindUnknown
	}
}

func ensureRequestLogLogger(logger *zap.Logger) *zap.Logger {
	if logger != nil {
		return logger
	}
	return zap.NewNop()
}
