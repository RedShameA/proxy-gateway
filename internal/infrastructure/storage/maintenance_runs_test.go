package storage

import (
	"context"
	"errors"
	"testing"

	maintenanceapp "proxygateway/internal/application/maintenance"
)

func TestMaintenanceRunRepositoryPersistsLifecycleAndQueries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, closeRepo := newMaintenanceRunRepositoryForTest(t)
	defer closeRepo()

	insertMaintenanceRunForTest(t, repo, maintenanceapp.Run{
		ID:            "run_profile_old",
		RunType:       maintenanceapp.RunTypeProfileEvaluation,
		TriggerSource: maintenanceapp.TriggerManual,
		TargetID:      "profile_1",
		TargetLabel:   "Primary profile",
		State:         maintenanceapp.StateQueued,
		TotalCount:    2,
		Detail:        map[string]any{"config_version": 1},
		CreatedAt:     1000,
		UpdatedAt:     1000,
	})
	insertMaintenanceRunForTest(t, repo, maintenanceapp.Run{
		ID:            "run_profile_new",
		RunType:       maintenanceapp.RunTypeProfileEvaluation,
		TriggerSource: maintenanceapp.TriggerScheduled,
		TargetID:      "profile_1",
		TargetLabel:   "Primary profile",
		State:         maintenanceapp.StateQueued,
		TotalCount:    3,
		Detail:        map[string]any{"config_version": 2},
		CreatedAt:     2000,
		UpdatedAt:     2000,
	})
	insertMaintenanceRunForTest(t, repo, maintenanceapp.Run{
		ID:            "run_subscription",
		RunType:       maintenanceapp.RunTypeSubscriptionRefresh,
		TriggerSource: maintenanceapp.TriggerManual,
		TargetID:      "sub_1",
		TargetLabel:   "Subscription",
		State:         maintenanceapp.StateQueued,
		TotalCount:    1,
		Detail:        map[string]any{"subscription_id": "sub_1"},
		CreatedAt:     1500,
		UpdatedAt:     1500,
	})

	loaded, err := repo.Load(ctx, "run_profile_old")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != maintenanceapp.StateQueued || loaded.TotalCount != 2 || loaded.Detail["config_version"] != float64(1) {
		t.Fatalf("loaded run = %#v", loaded)
	}

	claimed, ok, err := repo.ClaimNext(ctx, maintenanceapp.RunTypeProfileEvaluation, 3000)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected oldest queued profile evaluation to be claimed")
	}
	if claimed.ID != "run_profile_old" || claimed.State != maintenanceapp.StateRunning || claimed.StartedAt != 3000 {
		t.Fatalf("claimed run = %#v", claimed)
	}

	if err := repo.SetTotal(ctx, claimed.ID, 5, 3100); err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(ctx, maintenanceapp.FinishUpdate{
		ID:            claimed.ID,
		Result:        maintenanceapp.ResultSuccess,
		ReasonCode:    maintenanceapp.ReasonCompleted,
		FinishedCount: 5,
		Detail:        map[string]any{"selected_node_id": "node_1"},
		LastError:     "",
		NowMillis:     3200,
	}); err != nil {
		t.Fatal(err)
	}
	finished, err := repo.Load(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != maintenanceapp.StateFinished || finished.Result != maintenanceapp.ResultSuccess || finished.ReasonCode != maintenanceapp.ReasonCompleted {
		t.Fatalf("finished run state = %#v", finished)
	}
	if finished.TotalCount != 5 || finished.FinishedCount != 5 || finished.FinishedAt != 3200 {
		t.Fatalf("finished run counts/timestamps = %#v", finished)
	}
	if finished.Detail["selected_node_id"] != "node_1" {
		t.Fatalf("finished detail = %#v", finished.Detail)
	}

	listed, err := repo.List(ctx, maintenanceapp.ListFilter{RunType: maintenanceapp.RunTypeProfileEvaluation, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if listed.Total != 2 || len(listed.Items) != 2 || listed.Items[0].ID != "run_profile_new" || listed.Items[1].ID != "run_profile_old" {
		t.Fatalf("listed profile runs = %#v", listed)
	}

	events, err := repo.ListProfileEvents(ctx, "profile_1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].ID != "run_profile_new" || events[1].ID != "run_profile_old" {
		t.Fatalf("profile events = %#v", events)
	}

	unfinished, err := repo.ListUnfinished(ctx, maintenanceapp.RunTypeProfileEvaluation)
	if err != nil {
		t.Fatal(err)
	}
	if len(unfinished) != 1 || unfinished[0].ID != "run_profile_new" {
		t.Fatalf("unfinished profile runs = %#v", unfinished)
	}

	active, err := repo.ListActive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := maintenanceRunIDsForTest(active); !sameStringSet(got, []string{"run_profile_new", "run_subscription"}) {
		t.Fatalf("active run IDs = %#v", got)
	}

	claimedSubscription, ok, err := repo.ClaimNext(ctx, maintenanceapp.RunTypeSubscriptionRefresh, 3300)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || claimedSubscription.ID != "run_subscription" {
		t.Fatalf("claimed subscription run = %#v ok=%t", claimedSubscription, ok)
	}
	if err := repo.Start(ctx, "run_profile_new", 3400); err != nil {
		t.Fatal(err)
	}
	runningProfile, err := repo.Load(ctx, "run_profile_new")
	if err != nil {
		t.Fatal(err)
	}
	if runningProfile.State != maintenanceapp.StateRunning || runningProfile.StartedAt != 3400 {
		t.Fatalf("running profile run = %#v", runningProfile)
	}

	_, ok, err = repo.ClaimNext(ctx, maintenanceapp.RunTypeProfileEvaluation, 3500)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no queued profile evaluation after running the remaining profile run")
	}
}

func TestMaintenanceRunRepositoryReportsMissingRun(t *testing.T) {
	t.Parallel()

	repo, closeRepo := newMaintenanceRunRepositoryForTest(t)
	defer closeRepo()

	_, err := repo.Load(context.Background(), "run_missing")
	if !errors.Is(err, maintenanceapp.ErrRunNotFound) {
		t.Fatalf("Load missing error = %v, want ErrRunNotFound", err)
	}
}

func newMaintenanceRunRepositoryForTest(t *testing.T) (maintenanceapp.Repository, func()) {
	t.Helper()

	handle, err := Open(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), handle); err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	repo, err := NewMaintenanceRunRepository(handle)
	if err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	return repo, func() { _ = handle.Close() }
}

func insertMaintenanceRunForTest(t *testing.T, repo maintenanceapp.Repository, run maintenanceapp.Run) {
	t.Helper()

	if err := repo.Insert(context.Background(), run); err != nil {
		t.Fatal(err)
	}
}

func maintenanceRunIDsForTest(runs []maintenanceapp.Run) []string {
	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := map[string]int{}
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	return true
}
