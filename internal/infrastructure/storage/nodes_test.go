package storage

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	appnodes "proxygateway/internal/application/nodes"
	appobservations "proxygateway/internal/application/observations"
	appuow "proxygateway/internal/application/uow"
)

func TestNodeRepositoryContract(t *testing.T) {
	t.Parallel()

	for _, backend := range nodeRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			handle, repos, closeRepos := backend.open(t)
			defer closeRepos()
			testNodeRepositoryContract(t, handle, repos)
		})
	}
}

func TestPostgresNodeTransactionCommitAndRollbackVisibility(t *testing.T) {
	t.Parallel()

	handle, repos, closeRepos := newPostgresRepositoriesForTest(t)
	defer closeRepos()

	rollbackErr := errors.New("rollback node")
	err := handle.WithTx(context.Background(), func(tx appuow.Tx) error {
		_, err := appnodes.Service{NewNodeID: func() (string, error) {
			return "node_rollback", nil
		}}.Upsert(tx.NodeUpsertRepository(), appnodes.UpsertInput{
			Fingerprint:  "fp_rollback",
			Name:         "Rollback",
			Type:         "direct",
			OutboundJSON: `{"type":"direct"}`,
			SourceID:     "manual",
			SourceName:   "Manual",
			SourceType:   "manual",
			NowMillis:    1000,
		})
		if err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback error = %v, want rollbackErr", err)
	}
	if _, found, err := repos.Node.Load(context.Background(), "node_rollback"); err != nil || found {
		t.Fatalf("rolled back node found=%t err=%v", found, err)
	}

	if err := handle.WithTx(context.Background(), func(tx appuow.Tx) error {
		_, err := appnodes.Service{NewNodeID: func() (string, error) {
			return "node_commit", nil
		}}.Upsert(tx.NodeUpsertRepository(), appnodes.UpsertInput{
			Fingerprint:  "fp_commit",
			Name:         "Commit",
			Type:         "direct",
			OutboundJSON: `{"type":"direct"}`,
			SourceID:     "manual",
			SourceName:   "Manual",
			SourceType:   "manual",
			NowMillis:    1100,
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	node, found, err := repos.Node.Load(context.Background(), "node_commit")
	if err != nil {
		t.Fatal(err)
	}
	if !found || node.Name != "Commit" {
		t.Fatalf("committed node = %#v found=%t", node, found)
	}
}

func TestPostgresNodeCreateReturnsGeneratedID(t *testing.T) {
	t.Parallel()

	handle, repos, closeRepos := newPostgresRepositoriesForTest(t)
	defer closeRepos()

	service := appnodes.CreateService{
		Runner: NewTxRunners(handle),
		NewNodeID: func() (string, error) {
			return "node_generated_id", nil
		},
	}
	nodeID, err := service.Create(context.Background(), appnodes.CreateCommand{
		Node: appnodes.OutboundNode{
			Name:         "Generated ID",
			Type:         "direct",
			OutboundJSON: `{"type":"direct"}`,
		},
		Source: appnodes.SourceInput{
			ID:   "manual",
			Name: "Manual",
			Type: "manual",
		},
		NowMillis: 1200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if nodeID != "node_generated_id" {
		t.Fatalf("nodeID = %q, want generated ID", nodeID)
	}
	node, found, err := repos.Node.Load(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || node.Name != "Generated ID" {
		t.Fatalf("created node = %#v found=%t", node, found)
	}
}

func TestPostgresTransactionDoesNotExposeNestedRunner(t *testing.T) {
	t.Parallel()

	handle, _, closeRepos := newPostgresRepositoriesForTest(t)
	defer closeRepos()

	if err := handle.WithTx(context.Background(), func(tx appuow.Tx) error {
		if _, ok := any(tx).(appuow.Runner); ok {
			t.Fatal("postgres transaction should not expose nested WithTx")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNodeRepositoryManualUpdateAndDeleteContract(t *testing.T) {
	t.Parallel()

	for _, backend := range nodeRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			handle, repos, closeRepos := backend.open(t)
			defer closeRepos()

			upsertNodeForStorageTest(t, handle, "node_manual", "fp_manual", "Manual", "http", 100)

			updateService := appnodes.ManagementService{
				Repo:   repos.Node,
				Runner: NewTxRunners(handle),
				NewNodeID: func() (string, error) {
					return "node_unused", nil
				},
				Now: func() int64 { return 200 },
			}
			enabled := false
			result, err := updateService.UpdateManual(context.Background(), appnodes.ManualUpdateCommand{
				NodeID: "node_manual",
				Node: appnodes.OutboundNode{
					Name:         "Manual Updated",
					Type:         "socks5",
					OutboundJSON: `{"type":"socks"}`,
				},
				Enabled: &enabled,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.NodeID != "node_manual" || result.Split {
				t.Fatalf("manual update result = %#v", result)
			}
			loaded, found, err := repos.Node.Load(context.Background(), "node_manual")
			if err != nil {
				t.Fatal(err)
			}
			if !found || loaded.Name != "Manual Updated" || loaded.Type != "socks5" || loaded.Enabled {
				t.Fatalf("updated node = %#v found=%t", loaded, found)
			}
			sources, err := repos.Node.ListSources(context.Background(), "node_manual")
			if err != nil {
				t.Fatal(err)
			}
			if len(sources) != 1 || sources[0].DisplayName != "Manual Updated" {
				t.Fatalf("updated sources = %#v", sources)
			}

			deleteResult, err := appnodes.DeleteService{Runner: NewTxRunners(handle)}.DeleteManualSource(context.Background(), "node_manual")
			if err != nil {
				t.Fatal(err)
			}
			if len(deleteResult.DeletedFingerprints) != 1 {
				t.Fatalf("delete result = %#v", deleteResult)
			}
			if _, found, err := repos.Node.Load(context.Background(), "node_manual"); err != nil || found {
				t.Fatalf("deleted node found=%t err=%v", found, err)
			}
		})
	}
}

func testNodeRepositoryContract(t *testing.T, handle Handle, repos Repositories) {
	t.Helper()

	ctx := context.Background()

	upsertNodeForStorageTest(t, handle, "node_alpha", "fp_alpha", "Alpha", "direct", 100)
	upsertNodeForStorageTest(t, handle, "node_pending", "fp_pending", "Pending", "http", 101)
	upsertNodeForStorageTest(t, handle, "node_unusable", "fp_unusable", "Unusable", "socks5", 102)
	upsertNodeForStorageTest(t, handle, "node_disabled", "fp_disabled", "Disabled", "direct", 103)
	if _, err := repos.Node.SetEnabled(ctx, "node_disabled", false); err != nil {
		t.Fatal(err)
	}
	if err := repos.NodeObservation.SaveSuccess("node_alpha", appobservations.SuccessRecord{
		EgressIP:      "198.51.100.1",
		EgressCountry: "US",
		LatencyMS:     42,
	}, 1000); err != nil {
		t.Fatal(err)
	}
	if err := repos.NodeObservation.SaveSuccess("node_unusable", appobservations.SuccessRecord{
		EgressIP:      "198.51.100.2",
		EgressCountry: "JP",
		LatencyMS:     84,
	}, 1500); err != nil {
		t.Fatal(err)
	}
	if err := repos.NodeObservation.SaveFailure("node_unusable", "dial failed", 2000); err != nil {
		t.Fatal(err)
	}

	loaded, found, err := repos.Node.Load(ctx, "node_alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.ID != "node_alpha" || loaded.Name != "Alpha" || !loaded.Enabled || loaded.OutboundJSON != `{"type":"direct"}` {
		t.Fatalf("loaded = %#v found=%t", loaded, found)
	}
	if _, found, err := repos.Node.Load(ctx, "missing"); err != nil || found {
		t.Fatalf("missing load found=%t err=%v", found, err)
	}

	all, err := repos.Node.ListIDs(ctx, appnodes.ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 4 || !reflect.DeepEqual(all.IDs, []string{"node_alpha", "node_pending", "node_unusable", "node_disabled"}) {
		t.Fatalf("all = %#v", all)
	}

	nameMatch, err := repos.Node.ListIDs(ctx, appnodes.ListFilter{Name: "alpha", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if nameMatch.Total != 1 || nameMatch.IDs[0] != "node_alpha" {
		t.Fatalf("nameMatch = %#v", nameMatch)
	}

	usable := true
	usableMatch, err := repos.Node.ListIDs(ctx, appnodes.ListFilter{Usable: &usable, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if usableMatch.Total != 1 || usableMatch.IDs[0] != "node_alpha" {
		t.Fatalf("usableMatch = %#v", usableMatch)
	}

	unknownCountry, err := repos.Node.ListIDs(ctx, appnodes.ListFilter{EgressCountry: "__unknown__", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if unknownCountry.Total != 2 || !reflect.DeepEqual(unknownCountry.IDs, []string{"node_pending", "node_disabled"}) {
		t.Fatalf("unknownCountry = %#v", unknownCountry)
	}

	unusableState, err := repos.Node.ListIDs(ctx, appnodes.ListFilter{State: "unusable", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if unusableState.Total != 1 || unusableState.IDs[0] != "node_unusable" {
		t.Fatalf("unusableState = %#v", unusableState)
	}

	targets, err := repos.Node.ListEnabledObservationTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 3 || targets[0].ID != "node_alpha" || targets[2].ID != "node_unusable" {
		t.Fatalf("targets = %#v", targets)
	}

	sources, err := repos.Node.ListSources(ctx, "node_alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].SourceID != "manual" || sources[0].DisplayName != "Alpha" {
		t.Fatalf("sources = %#v", sources)
	}

	observation, found, err := repos.Node.LoadObservation(ctx, "node_alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !found || !observation.Usable || observation.EgressCountry != "US" || observation.LatencyMS != 42 {
		t.Fatalf("observation = %#v found=%t", observation, found)
	}

	updated, err := repos.Node.SetEnabled(ctx, "node_alpha", false)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("expected node_alpha enabled update")
	}
	loaded, found, err = repos.Node.Load(ctx, "node_alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Enabled {
		t.Fatalf("loaded after disable = %#v found=%t", loaded, found)
	}
	updated, err = repos.Node.SetEnabled(ctx, "missing", true)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("missing node should not be updated")
	}
}

type nodeRepositoryBackend struct {
	name     string
	parallel bool
	open     func(*testing.T) (Handle, Repositories, func())
}

func nodeRepositoryBackends(t *testing.T) []nodeRepositoryBackend {
	t.Helper()

	return []nodeRepositoryBackend{
		{name: "sqlite", parallel: true, open: newSQLiteRepositoriesForTest},
		{name: "postgres", open: newPostgresRepositoriesForTest},
	}
}

func newSQLiteRepositoriesForTest(t *testing.T) (Handle, Repositories, func()) {
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
	return handle, repos, func() { _ = handle.Close() }
}

func newRepositoriesForTest(t *testing.T) (Handle, Repositories, func()) {
	t.Helper()
	return newSQLiteRepositoriesForTest(t)
}

func newPostgresRepositoriesForTest(t *testing.T) (Handle, Repositories, func()) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("PROXYGATEWAY_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("PROXYGATEWAY_TEST_POSTGRES_DSN is not set")
	}
	handle, closeHandle := newIsolatedPostgresStorageHandleForTest(t, dsn)
	if err := Migrate(context.Background(), handle); err != nil {
		closeHandle()
		t.Fatal(err)
	}
	repos, err := NewRepositories(handle)
	if err != nil {
		closeHandle()
		t.Fatal(err)
	}
	return handle, repos, closeHandle
}

func upsertNodeForStorageTest(t *testing.T, handle Handle, id, fingerprint, name, nodeType string, nowMillis int64) {
	t.Helper()

	service := appnodes.Service{
		NewNodeID: func() (string, error) {
			return id, nil
		},
	}
	err := handle.WithTx(context.Background(), func(tx appuow.Tx) error {
		_, err := service.Upsert(tx.NodeUpsertRepository(), appnodes.UpsertInput{
			Fingerprint:  fingerprint,
			Name:         name,
			Type:         nodeType,
			OutboundJSON: `{"type":"` + outboundTypeForStorageTest(nodeType) + `"}`,
			SourceID:     "manual",
			SourceName:   "Manual",
			SourceType:   "manual",
			NowMillis:    nowMillis,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func outboundTypeForStorageTest(nodeType string) string {
	if nodeType == "socks5" {
		return "socks"
	}
	return nodeType
}
