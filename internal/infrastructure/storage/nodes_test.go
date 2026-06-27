package storage

import (
	"context"
	"reflect"
	"testing"

	appnodes "proxygateway/internal/application/nodes"
	appobservations "proxygateway/internal/application/observations"
	appuow "proxygateway/internal/application/uow"
)

func TestNodeRepositoryContract(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handle, repos, closeRepos := newRepositoriesForTest(t)
	defer closeRepos()

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

func newRepositoriesForTest(t *testing.T) (Handle, Repositories, func()) {
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
