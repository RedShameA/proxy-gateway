package proxy

import (
	"encoding/json"
	"testing"
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
