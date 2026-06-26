package app

import (
	"testing"
	"time"
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
	w := &requestLogWriter{
		events: make(chan requestLogEvent),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}

	if w.enqueue(requestLogEvent{kind: requestLogEventStart, id: "log_1"}) {
		t.Fatal("enqueue succeeded on full queue")
	}
	if got := w.droppedCount(); got != 1 {
		t.Fatalf("droppedCount = %d, want 1", got)
	}
}
