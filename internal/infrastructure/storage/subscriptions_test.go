package storage

import (
	"context"
	"testing"

	appsubscriptions "proxygateway/internal/application/subscriptions"
	appuow "proxygateway/internal/application/uow"
)

func TestSubscriptionRepositoryContract(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handle, repos, closeRepos := newRepositoriesForTest(t)
	defer closeRepos()

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
