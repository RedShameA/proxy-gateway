package proxy

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLogWriterDropsWhenQueueIsFull(t *testing.T) {
	core, observed := observer.New(zapcore.WarnLevel)
	w := &RequestLogWriter{
		events: make(chan requestLogEvent),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		logger: zap.New(core),
	}

	if w.enqueue(requestLogEvent{kind: requestLogEventStart, startRecord: RequestLogStartRecord{ID: "log_1"}}) {
		t.Fatal("enqueue succeeded on full queue")
	}
	if got := w.DroppedCount(); got != 1 {
		t.Fatalf("DroppedCount = %d, want 1", got)
	}
	if observed.Len() != 1 {
		t.Fatalf("observed log count = %d, want 1", observed.Len())
	}
	if got := observed.All()[0].Message; got != "request log event dropped" {
		t.Fatalf("observed message = %q, want request log event dropped", got)
	}
}

func TestRequestLogWriterLogsDatabaseWriteError(t *testing.T) {
	core, observed := observer.New(zapcore.ErrorLevel)
	w := &RequestLogWriter{
		repo:   failingRequestLogRepository{err: errors.New("write failed")},
		logger: zap.New(core),
	}

	w.write(requestLogEvent{
		kind:        requestLogEventStart,
		startRecord: RequestLogStartRecord{ID: "log_write_error", TargetHost: "example.test:443"},
	})

	if observed.Len() != 1 {
		t.Fatalf("observed log count = %d, want 1", observed.Len())
	}
	entry := observed.All()[0]
	if entry.Message != "write request log event failed" {
		t.Fatalf("observed message = %q, want write request log event failed", entry.Message)
	}
	fields := entry.ContextMap()
	if fields["event_kind"] != "start" || fields["log_id"] != "log_write_error" {
		t.Fatalf("observed fields = %#v, want event_kind=start and log_id=log_write_error", fields)
	}
}

type failingRequestLogRepository struct {
	err error
}

func (f failingRequestLogRepository) InsertStart(context.Context, RequestLogStartRecord) error {
	return f.err
}

func (f failingRequestLogRepository) Finish(context.Context, RequestLogFinishRecord) error {
	return f.err
}

func (f failingRequestLogRepository) InsertFailure(context.Context, RequestLogFailureRecord) error {
	return f.err
}

func (f failingRequestLogRepository) List(context.Context, RequestLogListFilter) (RequestLogListResult, error) {
	return RequestLogListResult{}, nil
}

func (f failingRequestLogRepository) CountSince(context.Context, int64) (RequestLogCounts, error) {
	return RequestLogCounts{}, nil
}

func (f failingRequestLogRepository) ListRecentFailures(context.Context, int) ([]RequestLogEntry, error) {
	return nil, nil
}

func (f failingRequestLogRepository) DeleteBefore(context.Context, int64) (int64, error) {
	return 0, nil
}
