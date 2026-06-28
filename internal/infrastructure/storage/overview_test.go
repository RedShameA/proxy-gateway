package storage

import (
	"context"
	"testing"

	databaseinfra "proxygateway/internal/infrastructure/database"
)

func TestOverviewRepositoryContract(t *testing.T) {
	t.Parallel()

	for _, backend := range nodeRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			handle, repos, closeRepos := backend.open(t)
			defer closeRepos()
			testOverviewRepositoryContract(t, handle, repos)
		})
	}
}

func testOverviewRepositoryContract(t *testing.T, handle Handle, repos Repositories) {
	t.Helper()
	if repos.Overview == nil {
		t.Fatal("overview repository is nil")
	}

	ctx := context.Background()
	insertOverviewFixture(t, handle)

	counts, err := repos.Overview.LoadResourceCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Subscriptions != 2 || counts.Nodes != 2 || counts.UsableNodes != 1 || counts.Profiles != 2 || counts.Credentials != 1 {
		t.Fatalf("counts = %#v", counts)
	}

	stateCounts, err := repos.Overview.LoadProfileStateCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stateCounts["ready"] != 1 || stateCounts["failed"] != 1 {
		t.Fatalf("stateCounts = %#v", stateCounts)
	}
}

func insertOverviewFixture(t *testing.T, handle Handle) {
	t.Helper()

	if handle.Dialect == databaseinfra.DialectPostgres {
		mustExecOverview(t, handle, `INSERT INTO subscriptions (id, name, source_type, created_at, updated_at) VALUES ('sub_1', 'sub 1', 'remote', 100, 100)`)
		mustExecOverview(t, handle, `INSERT INTO subscriptions (id, name, source_type, created_at, updated_at) VALUES ('sub_2', 'sub 2', 'local', 101, 101)`)
		mustExecOverview(t, handle, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_1', 'fp_1', 'node 1', 'direct', true, 100)`)
		mustExecOverview(t, handle, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_2', 'fp_2', 'node 2', 'direct', false, 101)`)
		mustExecOverview(t, handle, `INSERT INTO node_observations (node_id, usable, egress_country) VALUES ('node_1', true, 'US')`)
		mustExecOverview(t, handle, `INSERT INTO node_observations (node_id, usable, egress_country) VALUES ('node_2', true, 'JP')`)
		mustExecOverview(t, handle, `INSERT INTO access_profiles (id, profile_identifier, name, type, state, created_at) VALUES ('profile_1', 'p1', 'profile 1', 'fixed', 'ready', 100)`)
		mustExecOverview(t, handle, `INSERT INTO access_profiles (id, profile_identifier, name, type, state, created_at) VALUES ('profile_2', 'p2', 'profile 2', 'fastest', 'failed', 101)`)
		mustExecOverview(t, handle, `INSERT INTO proxy_credentials (id, profile_id, remark, password, password_hash, created_at) VALUES ('cred_1', 'profile_1', 'client', 'secret', 'hash', 100)`)
		return
	}

	mustExecOverview(t, handle, `INSERT INTO subscriptions (id, name, source_type, created_at, updated_at) VALUES ('sub_1', 'sub 1', 'remote', 100, 100)`)
	mustExecOverview(t, handle, `INSERT INTO subscriptions (id, name, source_type, created_at, updated_at) VALUES ('sub_2', 'sub 2', 'local', 101, 101)`)
	mustExecOverview(t, handle, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_1', 'fp_1', 'node 1', 'direct', 1, 100)`)
	mustExecOverview(t, handle, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_2', 'fp_2', 'node 2', 'direct', 0, 101)`)
	mustExecOverview(t, handle, `INSERT INTO node_observations (node_id, usable, egress_country) VALUES ('node_1', 1, 'US')`)
	mustExecOverview(t, handle, `INSERT INTO node_observations (node_id, usable, egress_country) VALUES ('node_2', 1, 'JP')`)
	mustExecOverview(t, handle, `INSERT INTO access_profiles (id, profile_identifier, name, type, state, created_at) VALUES ('profile_1', 'p1', 'profile 1', 'fixed', 'ready', 100)`)
	mustExecOverview(t, handle, `INSERT INTO access_profiles (id, profile_identifier, name, type, state, created_at) VALUES ('profile_2', 'p2', 'profile 2', 'fastest', 'failed', 101)`)
	mustExecOverview(t, handle, `INSERT INTO proxy_credentials (id, profile_id, remark, password, password_hash, created_at) VALUES ('cred_1', 'profile_1', 'client', 'secret', 'hash', 100)`)
}

func mustExecOverview(t *testing.T, handle Handle, query string, args ...any) {
	t.Helper()
	if _, err := handle.DB.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}
