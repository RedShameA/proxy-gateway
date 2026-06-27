package storage

import (
	"context"
	"testing"

	appproxy "proxygateway/internal/application/proxy"
)

func TestRequestLogRepositoryContract(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, closeRepo := newRequestLogRepositoryForTest(t)
	defer closeRepo()

	if err := repo.InsertStart(ctx, appproxy.RequestLogStartRecord{
		ID:                      "log_running",
		Timestamp:               1000,
		ProxyCredentialID:       "cred_1",
		ProxyCredential:         "client",
		AccessProfileID:         "profile_1",
		AccessProfile:           "profile",
		AccessProfileIdentifier: "profile_ident",
		TargetHost:              "example.test:443",
		ProxyPath:               "node",
		ProxyPathJSON:           `{"path_type":"single","node":{"id":"node_1"}}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(ctx, appproxy.RequestLogFinishRecord{
		ID:           "log_running",
		Success:      true,
		HTTPStatus:   204,
		DurationMS:   12,
		IngressBytes: 34,
		EgressBytes:  56,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertFailure(ctx, appproxy.RequestLogFailureRecord{
		ID:                      "log_failure",
		Timestamp:               2000,
		AccessProfile:           "profile_ident",
		AccessProfileIdentifier: "profile_ident",
		TargetHost:              "bad.test:443",
		FailureStage:            appproxy.FailureStageAuthentication,
		Error:                   "invalid credentials",
		HTTPStatus:              407,
		DurationMS:              5,
	}); err != nil {
		t.Fatal(err)
	}

	successes, err := repo.List(ctx, appproxy.RequestLogListFilter{Result: "success", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if successes.Total != 1 || len(successes.Items) != 1 {
		t.Fatalf("success result = total %d len %d", successes.Total, len(successes.Items))
	}
	success := successes.Items[0]
	if success.ID != "log_running" || success.Success == nil || !*success.Success || success.HTTPStatus != 204 {
		t.Fatalf("success item = %#v", success)
	}

	credentialMatches, err := repo.List(ctx, appproxy.RequestLogListFilter{Credential: "client", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if credentialMatches.Total != 1 || credentialMatches.Items[0].ID != "log_running" {
		t.Fatalf("credential filter result = %#v", credentialMatches)
	}

	nodeMatches, err := repo.List(ctx, appproxy.RequestLogListFilter{NodeID: "node_1", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if nodeMatches.Total != 1 || nodeMatches.Items[0].ID != "log_running" {
		t.Fatalf("node filter result = %#v", nodeMatches)
	}

	counts, err := repo.CountSince(ctx, 999)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 2 || counts.Failed != 1 {
		t.Fatalf("counts = %#v, want total=2 failed=1", counts)
	}

	recentFailures, err := repo.ListRecentFailures(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recentFailures) != 1 || recentFailures[0].ID != "log_failure" {
		t.Fatalf("recentFailures = %#v", recentFailures)
	}

	deleted, err := repo.DeleteBefore(ctx, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
}

func newRequestLogRepositoryForTest(t *testing.T) (appproxy.RequestLogRepository, func()) {
	t.Helper()

	handle, err := Open(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), handle); err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	repos, err := NewRepositories(handle)
	if err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	return repos.RequestLog, func() { _ = handle.Close() }
}
