package storage

import (
	"context"
	"errors"
	"testing"

	appnodes "proxygateway/internal/application/nodes"
	appsubscriptions "proxygateway/internal/application/subscriptions"
	appuow "proxygateway/internal/application/uow"
)

func TestSubscriptionRepositoryContract(t *testing.T) {
	t.Parallel()

	for _, backend := range nodeRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			handle, repos, closeRepos := backend.open(t)
			defer closeRepos()
			testSubscriptionRepositoryContract(t, handle, repos)
		})
	}
}

func TestPostgresSubscriptionRefreshImportRollsBackHalfWrittenChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handle, repos, closeRepos := newPostgresRepositoriesForTest(t)
	defer closeRepos()

	initialImport := appsubscriptions.ImportService{
		Runner: NewTxRunners(handle),
		NewNodeID: func() (string, error) {
			return "node_old", nil
		},
	}
	if _, _, err := initialImport.Import(ctx, appsubscriptions.ImportCommand{
		SubscriptionID: "sub_rollback",
		Name:           "Rollback",
		SourceType:     "local",
		Content:        "old",
		NowMillis:      1000,
	}, appsubscriptions.ParsedImportContent{
		Nodes: []appsubscriptions.ParsedNode{{
			Name:       "old-node",
			Type:       "http",
			Server:     "127.0.0.1",
			ServerPort: 18080,
		}},
		SkippedSummaryJSON: "[]",
	}); err != nil {
		t.Fatal(err)
	}

	nextIDs := []string{"node_new"}
	refreshImport := appsubscriptions.ImportService{
		Runner: NewTxRunners(handle),
		NewNodeID: func() (string, error) {
			id := nextIDs[0]
			nextIDs = nextIDs[1:]
			return id, nil
		},
	}
	_, _, err := refreshImport.Import(ctx, appsubscriptions.ImportCommand{
		SubscriptionID: "sub_rollback",
		Name:           "Rollback",
		SourceType:     "local",
		Content:        "new",
		Refresh:        true,
		NowMillis:      2000,
	}, appsubscriptions.ParsedImportContent{
		Nodes: []appsubscriptions.ParsedNode{
			{
				Name:       "new-node",
				Type:       "http",
				Server:     "127.0.0.1",
				ServerPort: 18081,
			},
			{
				Name:       "bad-node",
				Type:       "http",
				Server:     "127.0.0.1",
				ServerPort: 18082,
				TLSJSON:    []byte(`{`),
			},
		},
		SkippedSummaryJSON: "[]",
	})
	if err == nil {
		t.Fatal("expected refresh import to fail")
	}

	sub, found, err := repos.Subscription.Load(ctx, "sub_rollback")
	if err != nil {
		t.Fatal(err)
	}
	if !found || sub.Content != "old" || sub.UpdatedAt != 1000 {
		t.Fatalf("subscription after rollback = %#v found=%t, want old content", sub, found)
	}
	if _, found, err := repos.Node.Load(ctx, "node_new"); err != nil || found {
		t.Fatalf("half-imported node_new found=%t err=%v", found, err)
	}
	oldNode, found, err := repos.Node.Load(ctx, "node_old")
	if err != nil {
		t.Fatal(err)
	}
	if !found || oldNode.Name != "old-node" {
		t.Fatalf("old node after rollback = %#v found=%t", oldNode, found)
	}
}

func TestPostgresSubscriptionTransactionRollbackAndCommitVisibility(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handle, repos, closeRepos := newPostgresRepositoriesForTest(t)
	defer closeRepos()

	rollbackErr := errors.New("rollback subscription")
	err := handle.WithTx(ctx, func(tx appuow.Tx) error {
		if err := tx.SubscriptionImportRepository().CreateImport(appsubscriptions.ImportRecord{
			ID:                 "sub_tx_rollback",
			Name:               "Tx Rollback",
			SourceType:         "local",
			Content:            "rollback",
			SkippedSummaryJSON: "[]",
			NowMillis:          1000,
		}); err != nil {
			return err
		}
		nodeService := appnodes.Service{NewNodeID: func() (string, error) {
			return "node_tx_rollback", nil
		}}
		if _, err := nodeService.Upsert(tx.NodeUpsertRepository(), appnodes.UpsertInput{
			Fingerprint:  "fp_tx_rollback",
			Name:         "Tx Rollback",
			Type:         "direct",
			OutboundJSON: `{"type":"direct"}`,
			SourceID:     "sub_tx_rollback",
			SourceName:   "Tx Rollback",
			SourceType:   "subscription",
			NowMillis:    1000,
		}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback error = %v, want rollbackErr", err)
	}
	if _, found, err := repos.Subscription.Load(ctx, "sub_tx_rollback"); err != nil || found {
		t.Fatalf("rolled back subscription found=%t err=%v", found, err)
	}
	if _, found, err := repos.Node.Load(ctx, "node_tx_rollback"); err != nil || found {
		t.Fatalf("rolled back node found=%t err=%v", found, err)
	}

	if err := handle.WithTx(ctx, func(tx appuow.Tx) error {
		return tx.SubscriptionImportRepository().CreateImport(appsubscriptions.ImportRecord{
			ID:                 "sub_tx_commit",
			Name:               "Tx Commit",
			SourceType:         "local",
			Content:            "commit",
			SkippedSummaryJSON: "[]",
			NowMillis:          1100,
		})
	}); err != nil {
		t.Fatal(err)
	}
	if sub, found, err := repos.Subscription.Load(ctx, "sub_tx_commit"); err != nil || !found || sub.Name != "Tx Commit" {
		t.Fatalf("committed subscription = %#v found=%t err=%v", sub, found, err)
	}
}

func testSubscriptionRepositoryContract(t *testing.T, handle Handle, repos Repositories) {
	t.Helper()

	ctx := context.Background()

	if err := handle.WithTx(ctx, func(tx appuow.Tx) error {
		return tx.SubscriptionImportRepository().CreateImport(appsubscriptions.ImportRecord{
			ID:                         "sub_1",
			Name:                       "Remote",
			SourceType:                 "remote",
			URL:                        "https://example.test/sub",
			Content:                    "old",
			ImportedNodes:              2,
			SkippedEntries:             1,
			SkippedSummaryJSON:         `[{"reason":"malformed_entry","count":1}]`,
			AutoRefreshEnabled:         true,
			AutoRefreshIntervalSeconds: 600,
			NowMillis:                  1000,
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := handle.WithTx(ctx, func(tx appuow.Tx) error {
		return tx.SubscriptionImportRepository().CreateImport(appsubscriptions.ImportRecord{
			ID:                 "sub_2",
			Name:               "Local",
			SourceType:         "local",
			Content:            "local-content",
			ImportedNodes:      1,
			SkippedSummaryJSON: "[]",
			NowMillis:          1001,
		})
	}); err != nil {
		t.Fatal(err)
	}

	list, err := repos.Subscription.List(ctx, appsubscriptions.ListFilter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 2 || len(list.Items) != 1 || list.Items[0].ID != "sub_1" || !list.Items[0].AutoRefreshEnabled || list.Items[0].AutoRefreshIntervalSeconds != 600 {
		t.Fatalf("list = %#v", list)
	}

	loaded, found, err := repos.Subscription.Load(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Name != "Remote" || loaded.ImportedNodes != 2 || loaded.SkippedEntries != 1 || loaded.UpdatedAt != 1000 {
		t.Fatalf("loaded = %#v found=%t", loaded, found)
	}
	if _, found, err := repos.Subscription.Load(ctx, "missing"); err != nil || found {
		t.Fatalf("missing load found=%t err=%v", found, err)
	}

	importResult, found, err := repos.Subscription.LoadImportResult(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || importResult.ID != "sub_1" || importResult.ImportedNodes != 2 || importResult.SkippedEntries != 1 {
		t.Fatalf("importResult = %#v found=%t", importResult, found)
	}

	disabled := false
	interval := 0
	updated, err := repos.Subscription.UpdateAutoRefresh(ctx, "sub_1", &disabled, &interval, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("expected sub_1 update")
	}
	loaded, found, err = repos.Subscription.Load(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.AutoRefreshEnabled || loaded.AutoRefreshIntervalSeconds != 0 || loaded.UpdatedAt != 2000 {
		t.Fatalf("loaded after auto refresh update = %#v found=%t", loaded, found)
	}

	updated, err = repos.Subscription.UpdateAutoRefresh(ctx, "missing", &disabled, nil, 2100)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("missing subscription should not update")
	}

	if err := handle.WithTx(ctx, func(tx appuow.Tx) error {
		return tx.SubscriptionImportRepository().RefreshImport(appsubscriptions.ImportRecord{
			ID:                 "sub_1",
			Content:            "new",
			ImportedNodes:      3,
			SkippedEntries:     2,
			SkippedSummaryJSON: `[{"reason":"unsupported","count":2}]`,
			NowMillis:          3000,
		})
	}); err != nil {
		t.Fatal(err)
	}
	importResult, found, err = repos.Subscription.LoadImportResult(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || importResult.ImportedNodes != 3 || importResult.SkippedEntries != 2 {
		t.Fatalf("importResult after refresh = %#v found=%t", importResult, found)
	}

	if err := repos.Subscription.StoreRefreshError(ctx, "sub_1", "fetch failed", 4000); err != nil {
		t.Fatal(err)
	}
	loaded, found, err = repos.Subscription.Load(ctx, "sub_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.LastError != "fetch failed" || loaded.UpdatedAt != 4000 {
		t.Fatalf("loaded after error = %#v found=%t", loaded, found)
	}
}
