package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	maintenanceapp "proxygateway/internal/application/maintenance"
	postgresinfra "proxygateway/internal/infrastructure/postgres"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestMaintenanceRunRepositoryPersistsLifecycleAndQueries(t *testing.T) {
	t.Parallel()

	for _, backend := range maintenanceRunRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			repo, closeRepo := backend.open(t)
			defer closeRepo()
			testMaintenanceRunRepositoryPersistsLifecycleAndQueries(t, repo)
		})
	}
}

func testMaintenanceRunRepositoryPersistsLifecycleAndQueries(t *testing.T, repo maintenanceapp.Repository) {
	t.Helper()

	ctx := context.Background()

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

	for _, backend := range maintenanceRunRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			repo, closeRepo := backend.open(t)
			defer closeRepo()

			_, err := repo.Load(context.Background(), "run_missing")
			if !errors.Is(err, maintenanceapp.ErrRunNotFound) {
				t.Fatalf("Load missing error = %v, want ErrRunNotFound", err)
			}
		})
	}
}

func TestMaintenanceRunRepositoryListUsesStableOrderForCreatedAtTies(t *testing.T) {
	t.Parallel()

	for _, backend := range maintenanceRunRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			repo, closeRepo := backend.open(t)
			defer closeRepo()

			for _, id := range []string{"run_a", "run_b", "run_c"} {
				insertMaintenanceRunForTest(t, repo, maintenanceapp.Run{
					ID:            id,
					RunType:       maintenanceapp.RunTypeProfileEvaluation,
					TriggerSource: maintenanceapp.TriggerScheduled,
					State:         maintenanceapp.StateQueued,
					CreatedAt:     1000,
					UpdatedAt:     1000,
				})
			}

			firstPage, err := repo.List(context.Background(), maintenanceapp.ListFilter{RunType: maintenanceapp.RunTypeProfileEvaluation, Page: 1, PageSize: 2})
			if err != nil {
				t.Fatal(err)
			}
			secondPage, err := repo.List(context.Background(), maintenanceapp.ListFilter{RunType: maintenanceapp.RunTypeProfileEvaluation, Page: 2, PageSize: 2})
			if err != nil {
				t.Fatal(err)
			}
			if got := maintenanceRunIDsForTest(firstPage.Items); !sameStringSlice(got, []string{"run_c", "run_b"}) {
				t.Fatalf("first page IDs = %#v", got)
			}
			if got := maintenanceRunIDsForTest(secondPage.Items); !sameStringSlice(got, []string{"run_a"}) {
				t.Fatalf("second page IDs = %#v", got)
			}
		})
	}
}

func TestMaintenanceRunRepositoryClaimNextDoesNotDuplicateConcurrentPostgresClaims(t *testing.T) {
	t.Parallel()

	repo, closeRepo := newPostgresMaintenanceRunRepositoryForTest(t)
	defer closeRepo()

	const totalRuns = 20
	for i := range totalRuns {
		insertMaintenanceRunForTest(t, repo, maintenanceapp.Run{
			ID:            fmt.Sprintf("run_%02d", i),
			RunType:       maintenanceapp.RunTypeProfileEvaluation,
			TriggerSource: maintenanceapp.TriggerScheduled,
			State:         maintenanceapp.StateQueued,
			CreatedAt:     1000,
			UpdatedAt:     1000,
		})
	}

	claimed := make(chan string, totalRuns)
	errs := make(chan error, totalRuns)
	var wg sync.WaitGroup
	for i := range totalRuns {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			run, ok, err := repo.ClaimNext(context.Background(), maintenanceapp.RunTypeProfileEvaluation, int64(2000+worker))
			if err != nil {
				errs <- err
				return
			}
			if !ok {
				errs <- errors.New("expected queued run to be claimed")
				return
			}
			claimed <- run.ID
		}(i)
	}
	wg.Wait()
	close(claimed)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for id := range claimed {
		if seen[id] {
			t.Fatalf("run %s was claimed more than once", id)
		}
		seen[id] = true
	}
	if len(seen) != totalRuns {
		t.Fatalf("claimed %d runs, want %d", len(seen), totalRuns)
	}
}

func TestMaintenanceRunRepositoryStartupCleanupIsIdempotent(t *testing.T) {
	t.Parallel()

	for _, backend := range maintenanceRunRepositoryBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			if backend.parallel {
				t.Parallel()
			}
			repo, closeRepo := backend.open(t)
			defer closeRepo()

			insertMaintenanceRunForTest(t, repo, maintenanceapp.Run{
				ID:            "run_queued",
				RunType:       maintenanceapp.RunTypeProfileEvaluation,
				TriggerSource: maintenanceapp.TriggerScheduled,
				State:         maintenanceapp.StateQueued,
				FinishedCount: 1,
				Detail:        map[string]any{"profile_id": "profile_1"},
				CreatedAt:     1000,
				UpdatedAt:     1000,
			})
			insertMaintenanceRunForTest(t, repo, maintenanceapp.Run{
				ID:            "run_running",
				RunType:       maintenanceapp.RunTypeNodeObservation,
				TriggerSource: maintenanceapp.TriggerScheduled,
				State:         maintenanceapp.StateRunning,
				CreatedAt:     1100,
				UpdatedAt:     1100,
			})

			ids := []string{"run_startup_1", "run_startup_2"}
			service := maintenanceapp.NewService(repo, func(string) (string, error) {
				if len(ids) == 0 {
					return "", errors.New("unexpected id request")
				}
				id := ids[0]
				ids = ids[1:]
				return id, nil
			}, millisClockForStorageTest(2000, 2100, 2200, 2300, 2400, 2500))

			first, err := (maintenanceapp.StartupCleanupService{Runs: service}).Execute(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if first.CancelledCount != 2 {
				t.Fatalf("first cleanup cancelled %d runs, want 2", first.CancelledCount)
			}
			second, err := (maintenanceapp.StartupCleanupService{Runs: service}).Execute(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if second.CancelledCount != 0 {
				t.Fatalf("second cleanup cancelled %d runs, want 0", second.CancelledCount)
			}
			active, err := repo.ListActive(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(active) != 0 {
				t.Fatalf("active runs after idempotent cleanup = %#v", active)
			}
		})
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

type maintenanceRunRepositoryBackend struct {
	name     string
	parallel bool
	open     func(*testing.T) (maintenanceapp.Repository, func())
}

func maintenanceRunRepositoryBackends(t *testing.T) []maintenanceRunRepositoryBackend {
	t.Helper()

	return []maintenanceRunRepositoryBackend{
		{name: "sqlite", parallel: true, open: newMaintenanceRunRepositoryForTest},
		{name: "postgres", open: newPostgresMaintenanceRunRepositoryForTest},
	}
}

func newPostgresMaintenanceRunRepositoryForTest(t *testing.T) (maintenanceapp.Repository, func()) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("PROXYGATEWAY_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("PROXYGATEWAY_TEST_POSTGRES_DSN is not set")
	}
	handle, cleanup := newIsolatedPostgresStorageHandleForTest(t, dsn)
	if err := Migrate(context.Background(), handle); err != nil {
		cleanup()
		t.Fatal(err)
	}
	repo, err := NewMaintenanceRunRepository(handle)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	return repo, cleanup
}

func newIsolatedPostgresStorageHandleForTest(t *testing.T, dsn string) (Handle, func()) {
	t.Helper()

	base, err := sql.Open(postgresinfra.DriverName, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("proxygateway_storage_test_%d", time.Now().UnixNano())
	if _, err := base.ExecContext(context.Background(), `CREATE SCHEMA `+quoteIdentForStorageTest(schema)); err != nil {
		_ = base.Close()
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		_, _ = base.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+quoteIdentForStorageTest(schema)+` CASCADE`)
		_ = base.Close()
		t.Fatal(err)
	}
	if config.RuntimeParams == nil {
		config.RuntimeParams = map[string]string{}
	}
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	postgresinfra.ConfigureConnection(db)
	handle := Handle{DB: db, Dialect: "postgres"}
	cleanup := func() {
		_ = db.Close()
		_, _ = base.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+quoteIdentForStorageTest(schema)+` CASCADE`)
		_ = base.Close()
	}
	return handle, cleanup
}

func quoteIdentForStorageTest(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
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

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func millisClockForStorageTest(values ...int64) func() int64 {
	index := 0
	return func() int64 {
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}
