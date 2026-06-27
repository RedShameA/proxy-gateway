package proxy

import (
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestBuildRequestLogStartCapturesSingleNodePathSnapshot(t *testing.T) {
	record := BuildRequestLogStart(RequestLogStartInput{
		ID:         "log-1",
		Timestamp:  1234,
		TargetHost: "example.test:443",
		Credential: ProxyCredentialSnapshot{ID: "cred-1", Remark: "client"},
		Profile:    AccessProfileSnapshot{ID: "profile-1", Name: "profile", Identifier: "profile-ident"},
		Path: ProxyPathSnapshot{
			Node: NodeSnapshot{ID: "node-1", Name: "Node 1", Protocol: "http", Server: "127.0.0.1", ServerPort: 8080},
		},
	})

	if record.ID != "log-1" || record.Timestamp != 1234 || record.TargetHost != "example.test:443" {
		t.Fatalf("record identity = %#v", record)
	}
	if record.ProxyCredentialID != "cred-1" || record.ProxyCredential != "client" {
		t.Fatalf("credential = %#v", record)
	}
	if record.AccessProfileID != "profile-1" || record.AccessProfile != "profile" || record.AccessProfileIdentifier != "profile-ident" {
		t.Fatalf("profile = %#v", record)
	}
	if record.ProxyPath != "Node 1" {
		t.Fatalf("ProxyPath = %q", record.ProxyPath)
	}
	var path map[string]any
	if err := json.Unmarshal([]byte(record.ProxyPathJSON), &path); err != nil {
		t.Fatal(err)
	}
	if path["path_type"] != "single" {
		t.Fatalf("path = %#v", path)
	}
}

func TestBuildRequestLogFinishClearsFailureStageWhenSuccessful(t *testing.T) {
	record := BuildRequestLogFinish(RequestLogFinishInput{
		ID:           "log-1",
		Success:      true,
		FailureStage: "upstream",
		Error:        "ignored",
		HTTPStatus:   200,
		DurationMS:   25,
		IngressBytes: 10,
		EgressBytes:  20,
	})

	if !record.Success || record.FailureStage != "" || record.Error != "ignored" {
		t.Fatalf("record = %#v", record)
	}
	if record.HTTPStatus != 200 || record.DurationMS != 25 || record.IngressBytes != 10 || record.EgressBytes != 20 {
		t.Fatalf("metrics = %#v", record)
	}
}

func TestBuildRequestLogFailureCapturesStageAndTargetSnapshot(t *testing.T) {
	record := BuildRequestLogFailure(RequestLogFailureInput{
		ID:                "log-2",
		Timestamp:         2345,
		TargetHost:        "bad.example:443",
		ProfileIdentifier: "profile-ident",
		FailureStage:      "authentication",
		Error:             "invalid credentials",
		HTTPStatus:        407,
		DurationMS:        5,
	})

	if record.ID != "log-2" || record.TargetHost != "bad.example:443" || record.AccessProfileIdentifier != "profile-ident" {
		t.Fatalf("record = %#v", record)
	}
	if record.FailureStage != "authentication" || record.Error != "invalid credentials" || record.HTTPStatus != 407 || record.DurationMS != 5 {
		t.Fatalf("failure = %#v", record)
	}
}

func TestRequestLogServiceEnqueuesStartFinishAndFailure(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	sink := &recordingRequestLogSink{}
	ids := []string{"log_start", "log_failure"}
	service := NewRequestLogService(sink, func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}, zap.New(core))
	startedAt := time.UnixMilli(1234)
	path := SelectedPath{
		Credential:        CredentialRecord{ID: "cred-1", Remark: "client"},
		ProfileID:         "profile-1",
		Profile:           "profile",
		ProfileIdentifier: "profile-ident",
		Node:              Node{ID: "node-1", Name: "Node 1", Type: "http", Server: "127.0.0.1", ServerPort: 8080},
	}

	logID := service.Start(path, "example.test:443", startedAt)
	service.Finish(logID, true, "", "", 200, 12, 34, 56)
	service.RecordFailure("bad.example:443", "profile-ident", FailureStageAuthentication, "invalid proxy credentials", 407, startedAt)

	if logID != "log_start" {
		t.Fatalf("logID = %q, want log_start", logID)
	}
	if len(sink.starts) != 1 || sink.starts[0].ID != "log_start" || sink.starts[0].ProxyPath != "Node 1" {
		t.Fatalf("starts = %#v", sink.starts)
	}
	if len(sink.finishes) != 1 || sink.finishes[0].ID != "log_start" || !sink.finishes[0].Success {
		t.Fatalf("finishes = %#v", sink.finishes)
	}
	if len(sink.failures) != 1 || sink.failures[0].ID != "log_failure" || sink.failures[0].FailureStage != FailureStageAuthentication {
		t.Fatalf("failures = %#v", sink.failures)
	}
	if observed.FilterMessage("proxy request completed").Len() != 1 {
		t.Fatalf("expected one completed debug log, got %d", observed.FilterMessage("proxy request completed").Len())
	}
	if observed.FilterMessage("proxy request rejected").Len() != 1 {
		t.Fatalf("expected one rejected warn log, got %d", observed.FilterMessage("proxy request rejected").Len())
	}
}

type recordingRequestLogSink struct {
	starts   []RequestLogStartRecord
	finishes []RequestLogFinishRecord
	failures []RequestLogFailureRecord
}

func (s *recordingRequestLogSink) EnqueueStart(record RequestLogStartRecord) bool {
	s.starts = append(s.starts, record)
	return true
}

func (s *recordingRequestLogSink) EnqueueFinish(record RequestLogFinishRecord) bool {
	s.finishes = append(s.finishes, record)
	return true
}

func (s *recordingRequestLogSink) EnqueueFailure(record RequestLogFailureRecord) bool {
	s.failures = append(s.failures, record)
	return true
}
