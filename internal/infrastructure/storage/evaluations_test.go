package storage

import (
	"context"
	"reflect"
	"testing"

	appevaluations "proxygateway/internal/application/evaluations"
	appobservations "proxygateway/internal/application/observations"
	appprofiles "proxygateway/internal/application/profiles"
	domainprofile "proxygateway/internal/domain/profile"
)

func TestEvaluationRepositoryContract(t *testing.T) {
	t.Parallel()

	for _, backend := range nodeRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			handle, repos, closeRepos := backend.open(t)
			defer closeRepos()
			testEvaluationRepositoryContract(t, handle, repos)
		})
	}
}

func TestEvaluationRepositoryRetainedReleaseContract(t *testing.T) {
	t.Parallel()

	for _, backend := range nodeRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			handle, repos, closeRepos := backend.open(t)
			defer closeRepos()
			testEvaluationRepositoryRetainedReleaseContract(t, handle, repos)
		})
	}
}

func testEvaluationRepositoryContract(t *testing.T, handle Handle, repos Repositories) {
	t.Helper()
	if repos.Evaluation == nil {
		t.Fatal("evaluation repository is nil")
	}

	ctx := context.Background()
	fast := evaluationProfileForStorageTest("profile_fast", "fast", domainprofile.TypeFastest, 100)
	chain := evaluationProfileForStorageTest("profile_chain", "chain", domainprofile.TypeChain, 101)
	fixed := evaluationProfileForStorageTest("profile_fixed", "fixed", domainprofile.TypeFixedNode, 102)
	if err := repos.ProfileConfig.CreateConfig(ctx, fast, 100); err != nil {
		t.Fatal(err)
	}
	if err := repos.ProfileConfig.CreateConfig(ctx, chain, 101); err != nil {
		t.Fatal(err)
	}
	if err := repos.ProfileConfig.CreateConfig(ctx, fixed, 102); err != nil {
		t.Fatal(err)
	}

	targets, err := repos.Evaluation.ListTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets[0].ID != "profile_fast" || targets[1].ID != "profile_chain" {
		t.Fatalf("targets = %#v", targets)
	}
	target, found, err := repos.Evaluation.LoadTarget(ctx, "profile_chain")
	if err != nil {
		t.Fatal(err)
	}
	if !found || target.ID != "profile_chain" || target.Type != domainprofile.TypeChain || target.ExitNodeIDs[0] != "exit_1" || !target.NodeStickyEnabled {
		t.Fatalf("target = %#v found=%t", target, found)
	}
	if _, found, err := repos.Evaluation.LoadTarget(ctx, "missing"); err != nil || found {
		t.Fatalf("missing target found=%t err=%v", found, err)
	}
	if version, err := repos.Evaluation.CurrentConfigVersion(ctx, "profile_chain"); err != nil || version != 3 {
		t.Fatalf("version = %d err=%v, want 3", version, err)
	}

	insertCandidateNodeForEvaluationStorageTest(t, handle, repos, "node_b", "fp_b", "Node B", "direct", 200, "sub_1", "Remote")
	insertCandidateNodeForEvaluationStorageTest(t, handle, repos, "node_a", "fp_a", "Node A", "direct", 200, "sub_1", "Remote")
	insertCandidateNodeForEvaluationStorageTest(t, handle, repos, "node_disabled", "fp_disabled", "Disabled", "direct", 201, "sub_1", "Remote")
	if _, err := repos.Node.SetEnabled(ctx, "node_disabled", false); err != nil {
		t.Fatal(err)
	}
	insertRetainedProfileNodeForStorageTest(t, handle, "profile_chain", "node_retained_only")
	if err := repos.NodeObservation.SaveSuccess("node_a", appobservations.SuccessRecord{EgressCountry: "US", LatencyMS: 42}, 1000); err != nil {
		t.Fatal(err)
	}

	candidateIDs, err := repos.Evaluation.ListCandidateNodeIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(candidateIDs, []string{"node_b", "node_a"}) {
		t.Fatalf("candidateIDs = %#v", candidateIDs)
	}
	country, ok, err := repos.Evaluation.CandidateEgressCountry(ctx, "node_a")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || country != "US" {
		t.Fatalf("egress country = %q ok=%t, want US true", country, ok)
	}
	if _, ok, err := repos.Evaluation.CandidateEgressCountry(ctx, "node_b"); err != nil || ok {
		t.Fatalf("missing egress country ok=%t err=%v", ok, err)
	}
	refs, err := repos.Evaluation.ListCandidateSourceRefs(ctx, "node_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].SourceType != "subscription" || refs[0].SourceID != "sub_1" {
		t.Fatalf("refs = %#v", refs)
	}
	if retained, err := repos.Evaluation.ProfileRetainsNode(ctx, "profile_chain", "node_retained_only"); err != nil || !retained {
		t.Fatalf("retained = %t err=%v", retained, err)
	}
	if retained, err := repos.Evaluation.ProfileRetainsNode(ctx, "profile_chain", "node_a"); err != nil || retained {
		t.Fatalf("unexpected retained = %t err=%v", retained, err)
	}

	updated, err := repos.Evaluation.UpdateProfileState(ctx, "profile_chain", 2, appevaluations.StateUpdate{
		State: ptrEvaluationString(domainprofile.StateReady),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("stale config update succeeded")
	}
	updated, err = repos.Evaluation.UpdateProfileState(ctx, "profile_chain", 3, appevaluations.StateUpdate{
		State:                          ptrEvaluationString(domainprofile.StateReady),
		LastError:                      ptrEvaluationString(""),
		CurrentNodeID:                  ptrEvaluationString("node_a"),
		CurrentExitNodeID:              ptrEvaluationString("exit_1"),
		CurrentPathLatencyMS:           ptrEvaluationInt64(88),
		CurrentPathFailedEvaluations:   ptrEvaluationInt(0),
		CurrentPathMissedSuccessCycles: ptrEvaluationInt(0),
		SwitchReason:                   ptrEvaluationString(domainprofile.SwitchReasonCandidateClearlyBetter),
		LastEvaluationDetailsJSON:      ptrEvaluationString(`{"selected_node_id":"node_a"}`),
		LastEvaluatedAt:                ptrEvaluationInt64(2000),
		LastEvaluationStartedAt:        ptrEvaluationInt64(1900),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("fresh config update did not update")
	}
	if got, err := repos.Evaluation.CurrentNodeID(ctx, "profile_chain"); err != nil || got != "node_a" {
		t.Fatalf("current node = %q err=%v, want node_a", got, err)
	}
	path, err := repos.Evaluation.CurrentChainPath(ctx, "profile_chain")
	if err != nil {
		t.Fatal(err)
	}
	if path.FrontNodeID != "node_a" || path.ExitNodeID != "exit_1" {
		t.Fatalf("path = %#v", path)
	}
	counters, err := repos.Evaluation.CurrentPathCounters(ctx, "profile_chain")
	if err != nil {
		t.Fatal(err)
	}
	if counters.FailedEvaluations != 0 || counters.MissedSuccessCycles != 0 {
		t.Fatalf("counters = %#v", counters)
	}
	if latency, err := repos.Evaluation.CurrentPathLatency(ctx, "profile_chain"); err != nil || latency != 88 {
		t.Fatalf("latency = %d err=%v, want 88", latency, err)
	}
	if lastErr, err := repos.Evaluation.LastError(ctx, "profile_chain"); err != nil || lastErr != "" {
		t.Fatalf("last error = %q err=%v, want empty", lastErr, err)
	}
}

func testEvaluationRepositoryRetainedReleaseContract(t *testing.T, handle Handle, repos Repositories) {
	t.Helper()
	if repos.Evaluation == nil {
		t.Fatal("evaluation repository is nil")
	}

	ctx := context.Background()
	profile := evaluationProfileForStorageTest("profile_release", "release", domainprofile.TypeFastest, 100)
	if err := repos.ProfileConfig.CreateConfig(ctx, profile, 100); err != nil {
		t.Fatal(err)
	}
	insertRetainedProfileNodeForStorageTest(t, handle, "profile_release", "node_keep")
	insertRetainedProfileNodeForStorageTest(t, handle, "profile_release", "node_release")
	if err := repos.NodeObservation.SaveSuccess("node_release", appobservations.SuccessRecord{EgressCountry: "JP", LatencyMS: 70}, 1000); err != nil {
		t.Fatal(err)
	}

	result, err := repos.Evaluation.UpdateProfileStateAndReleaseRetained(ctx, "profile_release", 3, []string{"node_keep"}, true, appevaluations.StateUpdate{
		State:           ptrEvaluationString(domainprofile.StateReady),
		CurrentNodeID:   ptrEvaluationString("node_keep"),
		LastEvaluatedAt: ptrEvaluationInt64(3000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp_node_release" {
		t.Fatalf("DeletedFingerprints = %#v", result.DeletedFingerprints)
	}
	if _, found, err := repos.Node.Load(ctx, "node_release"); err != nil || found {
		t.Fatalf("released node found=%t err=%v", found, err)
	}
	if retained, err := repos.Evaluation.ProfileRetainsNode(ctx, "profile_release", "node_keep"); err != nil || !retained {
		t.Fatalf("kept retained = %t err=%v", retained, err)
	}
	if retained, err := repos.Evaluation.ProfileRetainsNode(ctx, "profile_release", "node_release"); err != nil || retained {
		t.Fatalf("released retained = %t err=%v", retained, err)
	}
}

func evaluationProfileForStorageTest(id, identifier, profileType string, createdAt int64) appprofiles.ConfigRecord {
	record := profileConfigRecordForStorageTest(id, identifier)
	record.Type = profileType
	record.CurrentNodeID = ""
	record.CurrentExitNodeID = ""
	record.State = domainprofile.StatePending
	record.SwitchReason = ""
	record.LastEvaluationDetailsJSON = "{}"
	if profileType == domainprofile.TypeFastest {
		record.ExitNodeIDs = nil
		record.ChainEvaluationMode = ""
	}
	return record
}

func insertCandidateNodeForEvaluationStorageTest(t *testing.T, handle Handle, repos Repositories, id, fingerprint, name, nodeType string, createdAt int64, sourceID, sourceName string) {
	t.Helper()
	upsertNodeForStorageTest(t, handle, id, fingerprint, name, nodeType, createdAt)
	execStorageSQL(t, handle,
		`UPDATE node_sources SET source_id = ?, source_name = ?, source_type = 'subscription', display_name = ? WHERE node_id = ?`,
		`UPDATE node_sources SET source_id = $1, source_name = $2, source_type = 'subscription', display_name = $3 WHERE node_id = $4`,
		sourceID,
		sourceName,
		name,
		id,
	)
}

func ptrEvaluationString(value string) *string {
	return &value
}

func ptrEvaluationInt(value int) *int {
	return &value
}

func ptrEvaluationInt64(value int64) *int64 {
	return &value
}
