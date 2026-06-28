package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	appmaintenance "proxygateway/internal/application/maintenance"
	appprofiles "proxygateway/internal/application/profiles"
	appproxy "proxygateway/internal/application/proxy"
	appuow "proxygateway/internal/application/uow"
	domainprofile "proxygateway/internal/domain/profile"
	databaseinfra "proxygateway/internal/infrastructure/database"
)

func TestProfileConfigRepositoryContract(t *testing.T) {
	t.Parallel()

	for _, backend := range nodeRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			handle, repos, closeRepos := backend.open(t)
			defer closeRepos()
			testProfileConfigRepositoryContract(t, handle, repos)
		})
	}
}

func TestProfileCredentialAndProxyAuthRepositoryContract(t *testing.T) {
	t.Parallel()

	for _, backend := range nodeRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			handle, repos, closeRepos := backend.open(t)
			defer closeRepos()
			testProfileCredentialAndProxyAuthRepositoryContract(t, handle, repos)
		})
	}
}

func TestProfileDeleteTransactionContract(t *testing.T) {
	t.Parallel()

	for _, backend := range nodeRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			handle, repos, closeRepos := backend.open(t)
			defer closeRepos()
			testProfileDeleteTransactionContract(t, handle, repos)
		})
	}
}

func TestPostgresProfileConfigTransactionRollbackAndCommitVisibility(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handle, repos, closeRepos := newPostgresRepositoriesForTest(t)
	defer closeRepos()

	record := profileConfigRecordForStorageTest("profile_tx", "profile-tx")
	if err := repos.ProfileConfig.CreateConfig(ctx, record, 1000); err != nil {
		t.Fatal(err)
	}

	rollbackErr := errors.New("rollback profile")
	err := handle.WithTx(ctx, func(tx appuow.Tx) error {
		updated := record
		updated.Name = "Rolled Back"
		updated.ConfigVersion = 2
		if err := tx.ProfileConfigRepository().UpdateConfig(ctx, updated, appprofiles.ConfigUpdateOptions{}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback error = %v, want rollbackErr", err)
	}
	loaded, found, err := repos.ProfileConfig.LoadConfig(ctx, "profile_tx")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Name != record.Name || loaded.ConfigVersion != record.ConfigVersion {
		t.Fatalf("profile after rollback = %#v found=%t", loaded, found)
	}

	if err := handle.WithTx(ctx, func(tx appuow.Tx) error {
		updated := record
		updated.Name = "Committed"
		updated.ConfigVersion = 2
		return tx.ProfileConfigRepository().UpdateConfig(ctx, updated, appprofiles.ConfigUpdateOptions{})
	}); err != nil {
		t.Fatal(err)
	}
	loaded, found, err = repos.ProfileConfig.LoadConfig(ctx, "profile_tx")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Name != "Committed" || loaded.ConfigVersion != 2 {
		t.Fatalf("profile after commit = %#v found=%t", loaded, found)
	}
}

func testProfileConfigRepositoryContract(t *testing.T, handle Handle, repos Repositories) {
	t.Helper()

	ctx := context.Background()
	record := profileConfigRecordForStorageTest("profile_1", "fast")
	if err := repos.ProfileConfig.CreateConfig(ctx, record, 1000); err != nil {
		t.Fatal(err)
	}
	list, err := repos.ProfileConfig.ListConfigIDs(ctx, appprofiles.ListConfigFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.IDs) != 1 || list.IDs[0] != "profile_1" {
		t.Fatalf("list = %#v", list)
	}
	loaded, found, err := repos.ProfileConfig.LoadConfig(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.ProfileIdentifier != "fast" || loaded.Type != domainprofile.TypeChain || loaded.NodeSourceMode != domainprofile.NodeSourceModeSpecificSubscriptions || loaded.ChainEvaluationMode != domainprofile.ChainEvaluationModeEndToEnd || loaded.EgressCountryMode != domainprofile.EgressCountryModeInclude {
		t.Fatalf("loaded profile = %#v found=%t", loaded, found)
	}
	if len(loaded.ExitNodeIDs) != 1 || loaded.ExitNodeIDs[0] != "exit_1" || len(loaded.EgressCountries) != 2 || loaded.SourceIDs[0] != "sub_1" || loaded.Protocols[0] != "direct" {
		t.Fatalf("loaded slice fields = %#v", loaded)
	}
	duplicate, err := repos.ProfileConfig.ProfileIdentifierExists(ctx, "fast", "profile_other")
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate {
		t.Fatal("expected duplicate profile identifier")
	}
	if exists, err := repos.ProfileConfig.Exists(ctx, "profile_1"); err != nil || !exists {
		t.Fatalf("profile exists = %t err=%v", exists, err)
	}

	setProfileEvaluationStateForStorageTest(t, handle, "profile_1")
	record.Name = "Fast Updated"
	record.SwitchReason = domainprofile.SwitchReasonAccessProfileChange
	record.LastEvaluationDetailsJSON = "{}"
	record.ConfigVersion = 4
	if err := repos.ProfileConfig.UpdateConfig(ctx, record, appprofiles.ConfigUpdateOptions{EvaluationChanged: true, ResetCurrentPath: true}); err != nil {
		t.Fatal(err)
	}
	loaded, found, err = repos.ProfileConfig.LoadConfig(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || loaded.Name != "Fast Updated" || loaded.ConfigVersion != 4 || loaded.LastError != "" || loaded.SwitchReason != domainprofile.SwitchReasonAccessProfileChange || loaded.LastEvaluatedAt != 0 {
		t.Fatalf("loaded after update = %#v found=%t", loaded, found)
	}
	failures, missed := profilePathCountersForStorageTest(t, handle, "profile_1")
	if failures != 0 || missed != 0 {
		t.Fatalf("path counters = %d/%d, want 0/0", failures, missed)
	}
}

func testProfileCredentialAndProxyAuthRepositoryContract(t *testing.T, handle Handle, repos Repositories) {
	t.Helper()

	ctx := context.Background()
	if err := repos.ProfileConfig.CreateConfig(ctx, profileConfigRecordForStorageTest("profile_1", "work"), 100); err != nil {
		t.Fatal(err)
	}
	exists, err := repos.ProfileCredential.ProfileExists(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("expected profile_1 to exist")
	}
	identifier, found, err := repos.ProfileCredential.LoadProfileIdentifier(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || identifier != "work" {
		t.Fatalf("identifier = %q found=%t", identifier, found)
	}
	if err := repos.ProfileCredential.CreateCredential(ctx, appprofiles.CredentialCreateRecord{
		ID:        "cred_1",
		ProfileID: "profile_1",
		Remark:    "client",
		Password:  "secret",
		CreatedAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.ProfileCredential.CreateCredential(ctx, appprofiles.CredentialCreateRecord{
		ID:        "cred_disabled",
		ProfileID: "profile_1",
		Remark:    "disabled",
		Password:  "disabled",
		CreatedAt: 1001,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.ProfileCredential.SetCredentialEnabled(ctx, "profile_1", "cred_disabled", false); err != nil {
		t.Fatal(err)
	}
	duplicate, err := repos.ProfileCredential.PasswordExists(ctx, "profile_1", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate {
		t.Fatal("expected duplicate credential password")
	}
	records, err := repos.ProfileCredential.ListCredentials(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ID != "cred_1" || records[0].Remark != "client" || !records[0].Enabled || records[1].Enabled {
		t.Fatalf("credentials = %#v", records)
	}
	counts, err := repos.ProfileCredential.CountCredentials(ctx, "profile_1")
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 2 || counts.Enabled != 1 {
		t.Fatalf("counts = %#v", counts)
	}

	record, profileFound, credentialFound, err := repos.ProxyCredential.LookupCredential(ctx, "work", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !profileFound || !credentialFound || record.ID != "cred_1" || record.Remark != "client" || record.ProfileID != "profile_1" {
		t.Fatalf("lookup success = %#v profileFound=%t credentialFound=%t", record, profileFound, credentialFound)
	}
	if _, profileFound, credentialFound, err := repos.ProxyCredential.LookupCredential(ctx, "missing", "secret"); err != nil || profileFound || credentialFound {
		t.Fatalf("missing profile: profileFound=%t credentialFound=%t err=%v", profileFound, credentialFound, err)
	}
	if _, profileFound, credentialFound, err := repos.ProxyCredential.LookupCredential(ctx, "work", "disabled"); err != nil || !profileFound || credentialFound {
		t.Fatalf("disabled credential: profileFound=%t credentialFound=%t err=%v", profileFound, credentialFound, err)
	}
	if err := repos.ProxyCredential.TouchCredentialLastUsed(ctx, "cred_1", 2000, 1500); err != nil {
		t.Fatal(err)
	}
	if got := credentialLastUsedForStorageTest(t, handle, "cred_1"); got != 2000 {
		t.Fatalf("last_used_at = %d, want 2000", got)
	}
	if err := repos.ProxyCredential.TouchCredentialLastUsed(ctx, "cred_1", 3000, 1500); err != nil {
		t.Fatal(err)
	}
	if got := credentialLastUsedForStorageTest(t, handle, "cred_1"); got != 2000 {
		t.Fatalf("recent last_used_at = %d, want unchanged 2000", got)
	}

	deleted, err := repos.ProfileCredential.DeleteCredential(ctx, "profile_1", "cred_1")
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected credential delete")
	}
	if _, profileFound, credentialFound, err := repos.ProxyCredential.LookupCredential(ctx, "work", "secret"); err != nil || !profileFound || credentialFound {
		t.Fatalf("deleted credential lookup: profileFound=%t credentialFound=%t err=%v", profileFound, credentialFound, err)
	}

	var _ appproxy.CredentialRepository = repos.ProxyCredential
}

func testProfileDeleteTransactionContract(t *testing.T, handle Handle, repos Repositories) {
	t.Helper()

	ctx := context.Background()
	if err := repos.ProfileConfig.CreateConfig(ctx, profileConfigRecordForStorageTest("profile_delete", "delete-me"), 100); err != nil {
		t.Fatal(err)
	}
	if err := repos.ProfileCredential.CreateCredential(ctx, appprofiles.CredentialCreateRecord{
		ID:        "cred_delete",
		ProfileID: "profile_delete",
		Remark:    "client",
		Password:  "secret",
		CreatedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.MaintenanceRun.Insert(ctx, appmaintenance.Run{
		ID:            "run_delete",
		RunType:       appmaintenance.RunTypeProfileEvaluation,
		TriggerSource: appmaintenance.TriggerManual,
		TargetID:      "profile_delete",
		State:         appmaintenance.StateFinished,
		CreatedAt:     100,
		UpdatedAt:     100,
	}); err != nil {
		t.Fatal(err)
	}
	insertRetainedProfileNodeForStorageTest(t, handle, "profile_delete", "node_retained")

	result, err := appprofiles.DeleteService{Runner: NewTxRunners(handle)}.Delete(ctx, "profile_delete")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DeletedFingerprints) != 1 {
		t.Fatalf("delete result = %#v, want one deleted retained node", result)
	}
	for _, item := range []struct {
		name  string
		table string
		where string
		value string
	}{
		{name: "profile", table: "access_profiles", where: "id", value: "profile_delete"},
		{name: "credentials", table: "proxy_credentials", where: "profile_id", value: "profile_delete"},
		{name: "runs", table: "maintenance_runs", where: "target_id", value: "profile_delete"},
		{name: "retained", table: "retained_profile_nodes", where: "profile_id", value: "profile_delete"},
		{name: "retained_node", table: "nodes", where: "id", value: "node_retained"},
	} {
		if count := countRowsForStorageTest(t, handle, item.table, item.where, item.value); count != 0 {
			t.Fatalf("%s count = %d, want 0", item.name, count)
		}
	}
}

func profileConfigRecordForStorageTest(id, identifier string) appprofiles.ConfigRecord {
	return appprofiles.ConfigRecord{
		ID:                           id,
		Name:                         "Fast",
		ProfileIdentifier:            identifier,
		Type:                         domainprofile.TypeChain,
		FixedNodeID:                  "front_1",
		ExitNodeIDs:                  []string{"exit_1"},
		ChainEvaluationMode:          domainprofile.ChainEvaluationModeEndToEnd,
		TestURL:                      "https://example.test/204",
		EgressCountry:                "US",
		EgressCountryMode:            domainprofile.EgressCountryModeInclude,
		EgressCountries:              []string{"US", "JP"},
		NodeSourceMode:               domainprofile.NodeSourceModeSpecificSubscriptions,
		SourceIDs:                    []string{"sub_1"},
		Protocols:                    []string{"direct"},
		NameIncludeRegex:             "tokyo",
		MinEvaluationIntervalSeconds: 10,
		CandidateLimit:               20,
		RelativeImprovementThreshold: 0.3,
		AbsoluteLatencyImprovementMS: 50,
		CurrentNodeID:                "node_1",
		CurrentExitNodeID:            "exit_1",
		State:                        domainprofile.StateReady,
		AutoEvaluationEnabled:        true,
		AutoEvaluationInterval:       60,
		NodeStickyEnabled:            true,
		ConfigVersion:                3,
	}
}

func setProfileEvaluationStateForStorageTest(t *testing.T, handle Handle, profileID string) {
	t.Helper()
	execStorageSQL(t, handle,
		`UPDATE access_profiles SET last_error = 'old error', current_path_failed_evaluations = 2, current_path_missed_success_cycles = 3, switch_reason = 'old', last_evaluation_details_json = '{"old":true}', last_evaluated_at = 900, last_evaluation_started_at = 800 WHERE id = ?`,
		`UPDATE access_profiles SET last_error = 'old error', current_path_failed_evaluations = 2, current_path_missed_success_cycles = 3, switch_reason = 'old', last_evaluation_details_json = '{"old":true}', last_evaluated_at = 900, last_evaluation_started_at = 800 WHERE id = $1`,
		profileID,
	)
}

func profilePathCountersForStorageTest(t *testing.T, handle Handle, profileID string) (int, int) {
	t.Helper()
	var failures, missed int
	if err := queryRowStorageSQL(t, handle,
		`SELECT current_path_failed_evaluations, current_path_missed_success_cycles FROM access_profiles WHERE id = ?`,
		`SELECT current_path_failed_evaluations, current_path_missed_success_cycles FROM access_profiles WHERE id = $1`,
		profileID,
	).Scan(&failures, &missed); err != nil {
		t.Fatal(err)
	}
	return failures, missed
}

func credentialLastUsedForStorageTest(t *testing.T, handle Handle, credentialID string) int64 {
	t.Helper()
	var lastUsed int64
	if err := queryRowStorageSQL(t, handle,
		`SELECT last_used_at FROM proxy_credentials WHERE id = ?`,
		`SELECT last_used_at FROM proxy_credentials WHERE id = $1`,
		credentialID,
	).Scan(&lastUsed); err != nil {
		t.Fatal(err)
	}
	return lastUsed
}

func insertRetainedProfileNodeForStorageTest(t *testing.T, handle Handle, profileID, nodeID string) {
	t.Helper()
	execStorageSQL(t, handle,
		`INSERT INTO nodes (id, fingerprint, name, type, created_at) VALUES (?, ?, 'Retained', 'direct', 100)`,
		`INSERT INTO nodes (id, fingerprint, name, type, created_at) VALUES ($1, $2, 'Retained', 'direct', 100)`,
		nodeID,
		"fp_"+nodeID,
	)
	execStorageSQL(t, handle,
		`INSERT INTO retained_profile_nodes (profile_id, node_id, created_at) VALUES (?, ?, 100)`,
		`INSERT INTO retained_profile_nodes (profile_id, node_id, created_at) VALUES ($1, $2, 100)`,
		profileID,
		nodeID,
	)
}

func countRowsForStorageTest(t *testing.T, handle Handle, table, column, value string) int {
	t.Helper()
	var count int
	switch handle.Dialect {
	case "", databaseinfra.DialectSQLite:
		if err := queryRowStorageSQL(t, handle, `SELECT COUNT(*) FROM `+table+` WHERE `+column+` = ?`, "", value).Scan(&count); err != nil {
			t.Fatal(err)
		}
	case databaseinfra.DialectPostgres:
		if err := queryRowStorageSQL(t, handle, "", `SELECT COUNT(*) FROM `+table+` WHERE `+column+` = $1`, value).Scan(&count); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unsupported dialect %q", handle.Dialect)
	}
	return count
}

func execStorageSQL(t *testing.T, handle Handle, sqliteSQL, postgresSQL string, args ...any) {
	t.Helper()
	query := dialectSQLForStorageTest(t, handle, sqliteSQL, postgresSQL)
	if _, err := handle.DB.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func queryRowStorageSQL(t *testing.T, handle Handle, sqliteSQL, postgresSQL string, args ...any) *sql.Row {
	t.Helper()
	query := dialectSQLForStorageTest(t, handle, sqliteSQL, postgresSQL)
	return handle.DB.QueryRow(query, args...)
}

func dialectSQLForStorageTest(t *testing.T, handle Handle, sqliteSQL, postgresSQL string) string {
	t.Helper()
	switch handle.Dialect {
	case "", databaseinfra.DialectSQLite:
		return sqliteSQL
	case databaseinfra.DialectPostgres:
		return postgresSQL
	default:
		t.Fatalf("unsupported dialect %q", handle.Dialect)
		return ""
	}
}
