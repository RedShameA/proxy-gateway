package app

import (
	"testing"
	"time"

	sqliteinfra "proxygateway/internal/infrastructure/sqlite"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestAsyncRequestLogWriterFlushesQueuedEvents(t *testing.T) {
	g, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = g.Close() }()

	path := selectedProxyPath{
		Credential:        proxyCredentialRecord{ID: "cred_1", Remark: "client"},
		ProfileID:         "profile_1",
		Profile:           "profile",
		ProfileIdentifier: "profile_1",
		Node:              nodeRecord{ID: "node_1", Name: "node", Type: "direct"},
	}
	logID := g.startProxyRequest(path, "example.test:443", time.Now())
	if logID == "" {
		t.Fatal("startProxyRequest returned empty log id")
	}
	g.finishProxyRequest(logID, true, "", "", 200, 12, 34, 56)

	if ok := g.requestLogs.close(time.Second); !ok {
		t.Fatal("request log writer did not flush")
	}

	var state string
	var success int
	var durationMS, ingressBytes, egressBytes int64
	if err := g.db.QueryRow(
		`SELECT state, success, duration_ms, ingress_bytes, egress_bytes FROM request_logs WHERE id = ?`,
		logID,
	).Scan(&state, &success, &durationMS, &ingressBytes, &egressBytes); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || success != 1 || durationMS != 12 || ingressBytes != 34 || egressBytes != 56 {
		t.Fatalf("request log = state %s success %d duration %d ingress %d egress %d", state, success, durationMS, ingressBytes, egressBytes)
	}
}

func TestRequestLogWriterDropsWhenQueueIsFull(t *testing.T) {
	core, observed := observer.New(zapcore.WarnLevel)
	w := &requestLogWriter{
		events: make(chan requestLogEvent),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		logger: zap.New(core),
	}

	if w.enqueue(requestLogEvent{kind: requestLogEventStart, id: "log_1"}) {
		t.Fatal("enqueue succeeded on full queue")
	}
	if got := w.droppedCount(); got != 1 {
		t.Fatalf("droppedCount = %d, want 1", got)
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
	db, err := sqliteinfra.Open(sqliteinfra.DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	w := &requestLogWriter{
		db:     db,
		logger: zap.New(core),
	}

	w.write(requestLogEvent{kind: requestLogEventStart, id: "log_write_error", targetHost: "example.test:443"})

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
