package sqlite

import (
	"context"
	"reflect"
	"testing"

	appnodes "proxygateway/internal/application/nodes"
	appobservations "proxygateway/internal/application/observations"
)

func TestNodeRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := NewNodeRepository(db)

	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, server, server_port, username, password, raw_json, outbound_json, enabled, created_at) VALUES ('node_alpha', 'fp_alpha', 'Alpha', 'direct', '', 0, '', '', '{}', '{"type":"direct"}', 1, 100)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_pending', 'fp_pending', 'Pending', 'http', 1, 101)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_unusable', 'fp_unusable', 'Unusable', 'socks5', 1, 102)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_disabled', 'fp_disabled', 'Disabled', 'direct', 0, 103)`)
	mustExec(t, db, `INSERT INTO nodes (id, fingerprint, name, type, enabled, created_at) VALUES ('node_retained', 'fp_retained', 'Retained', 'direct', 1, 104)`)
	mustExec(t, db, `INSERT INTO node_sources (node_id, source_id, source_name, source_type, display_name, created_at) VALUES ('node_alpha', 'sub_alpha', 'Remote Alpha', 'subscription', 'Alpha Display', 100)`)
	mustExec(t, db, `INSERT INTO retained_profile_nodes (profile_id, node_id, created_at) VALUES ('profile_1', 'node_retained', 100)`)
	mustExec(t, db, `INSERT INTO node_observations (node_id, usable, egress_ip, egress_country, latency_ms, last_success_at) VALUES ('node_alpha', 1, '198.51.100.1', 'US', 42, 1000)`)
	mustExec(t, db, `INSERT INTO node_observations (node_id, usable, egress_country, last_error, last_failure_at) VALUES ('node_unusable', 0, 'JP', 'dial failed', 2000)`)

	loaded, found, err := repo.Load(ctx, "node_alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.ID != "node_alpha" || loaded.Name != "Alpha" || !loaded.Enabled || loaded.OutboundJSON != `{"type":"direct"}` {
		t.Fatalf("loaded = %#v found=%t", loaded, found)
	}
	if _, found, err := repo.Load(ctx, "missing"); err != nil || found {
		t.Fatalf("missing load found=%t err=%v", found, err)
	}

	all, err := repo.ListIDs(ctx, appnodes.ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 4 || !reflect.DeepEqual(all.IDs, []string{"node_alpha", "node_pending", "node_unusable", "node_disabled"}) {
		t.Fatalf("all = %#v", all)
	}
	sourceMatch, err := repo.ListIDs(ctx, appnodes.ListFilter{Name: "remote alpha", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if sourceMatch.Total != 1 || sourceMatch.IDs[0] != "node_alpha" {
		t.Fatalf("sourceMatch = %#v", sourceMatch)
	}
	usable := true
	usableMatch, err := repo.ListIDs(ctx, appnodes.ListFilter{Usable: &usable, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if usableMatch.Total != 1 || usableMatch.IDs[0] != "node_alpha" {
		t.Fatalf("usableMatch = %#v", usableMatch)
	}
	unknownCountry, err := repo.ListIDs(ctx, appnodes.ListFilter{EgressCountry: "__unknown__", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if unknownCountry.Total != 2 || !reflect.DeepEqual(unknownCountry.IDs, []string{"node_pending", "node_disabled"}) {
		t.Fatalf("unknownCountry = %#v", unknownCountry)
	}
	unusableState, err := repo.ListIDs(ctx, appnodes.ListFilter{State: "unusable", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if unusableState.Total != 1 || unusableState.IDs[0] != "node_unusable" {
		t.Fatalf("unusableState = %#v", unusableState)
	}

	targets, err := repo.ListEnabledObservationTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 4 || targets[0].ID != "node_alpha" || targets[3].ID != "node_retained" {
		t.Fatalf("targets = %#v", targets)
	}

	sources, err := repo.ListSources(ctx, "node_alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].SourceID != "sub_alpha" || sources[0].DisplayName != "Alpha Display" {
		t.Fatalf("sources = %#v", sources)
	}

	observation, found, err := repo.LoadObservation(ctx, "node_alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !found || !observation.Usable || observation.EgressCountry != "US" || observation.LatencyMS != 42 {
		t.Fatalf("observation = %#v found=%t", observation, found)
	}

	updated, err := repo.SetEnabled(ctx, "node_alpha", false)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("expected node_alpha enabled update")
	}
	loaded, found, err = repo.Load(ctx, "node_alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Enabled {
		t.Fatalf("loaded after disable = %#v found=%t", loaded, found)
	}
	updated, err = repo.SetEnabled(ctx, "missing", true)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("missing node should not be updated")
	}
}

func TestNodeObservationRepositoryContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	nodeRepo := NewNodeRepository(db)
	observationRepo := NewNodeObservationRepository(db)

	if err := observationRepo.SaveSuccess("node_1", appobservations.SuccessRecord{
		EgressIP:      "198.51.100.10",
		EgressCountry: "US",
		LatencyMS:     33,
	}, 1000); err != nil {
		t.Fatal(err)
	}
	observation, found, err := nodeRepo.LoadObservation(ctx, "node_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || !observation.Usable || observation.EgressIP != "198.51.100.10" || observation.LastSuccessAt != 1000 {
		t.Fatalf("success observation = %#v found=%t", observation, found)
	}

	if err := observationRepo.SaveFailure("node_1", "dial failed", 2000); err != nil {
		t.Fatal(err)
	}
	observation, found, err = nodeRepo.LoadObservation(ctx, "node_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || observation.Usable || observation.LastError != "dial failed" || observation.LastFailureAt != 2000 {
		t.Fatalf("failure observation = %#v found=%t", observation, found)
	}
}

func TestNodeTransactionRepositoriesContract(t *testing.T) {
	db, err := Open(DefaultPath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	nodeRepo := NewNodeRepository(db)
	service := appnodes.Service{
		NewNodeID: func() (string, error) {
			return "node_created", nil
		},
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := service.Upsert(NewNodeUpsertRepositoryTx(tx), appnodes.UpsertInput{
		Fingerprint:  "fp_created",
		Name:         "Manual",
		Type:         "http",
		Server:       "127.0.0.1",
		ServerPort:   1080,
		Username:     "user",
		Password:     "pass",
		OutboundJSON: `{"type":"http"}`,
		SourceID:     "manual",
		SourceName:   "Manual",
		SourceType:   "manual",
		NowMillis:    1000,
	})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if nodeID != "node_created" {
		t.Fatalf("nodeID = %q", nodeID)
	}
	loaded, found, err := nodeRepo.Load(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Name != "Manual" || loaded.ServerPort != 1080 {
		t.Fatalf("loaded = %#v found=%t", loaded, found)
	}
	sources, err := nodeRepo.ListSources(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 || sources[0].SourceType != "manual" {
		t.Fatalf("sources = %#v", sources)
	}

	disabled := false
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.UpdateManual(NewNodeManualUpdateRepositoryTx(tx), appnodes.ManualUpdateInput{
		NodeID:       nodeID,
		Fingerprint:  "fp_updated",
		Name:         "Manual Updated",
		Type:         "socks5",
		Server:       "127.0.0.2",
		ServerPort:   1081,
		OutboundJSON: `{"type":"socks"}`,
		Enabled:      &disabled,
		NowMillis:    2000,
	})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if result.NodeID != nodeID || result.Split {
		t.Fatalf("result = %#v", result)
	}
	loaded, found, err = nodeRepo.Load(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Name != "Manual Updated" || loaded.Enabled || loaded.Type != "socks5" {
		t.Fatalf("loaded after update = %#v found=%t", loaded, found)
	}

	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	affected, err := NewNodeDeleteRepositoryTx(tx).DeleteManualSource(nodeID)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("affected = %d, want 1", affected)
	}
	sources, err = nodeRepo.ListSources(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("sources after delete = %#v", sources)
	}
}
