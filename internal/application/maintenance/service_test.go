package maintenance

import (
	"context"
	"reflect"
	"testing"
)

func TestServiceCreatesStartsAndFinishesMaintenanceRun(t *testing.T) {
	store := newMemoryRepository()
	var nextID string
	service := NewService(store, func(prefix string) (string, error) {
		nextID = prefix + "_test"
		return nextID, nil
	}, millisClock(1000, 1100, 1200, 1300))

	created, err := service.Create(context.Background(), CreateCommand{
		RunType:       RunTypeProfileEvaluation,
		TriggerSource: TriggerManual,
		TargetID:      "profile_1",
		TargetLabel:   "Work",
		TotalCount:    -5,
		Detail:        map[string]any{"profile_id": "profile_1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != nextID || created.State != StateQueued || created.TotalCount != 0 || created.CreatedAt != 1000 || created.UpdatedAt != 1000 {
		t.Fatalf("created run = %+v", created)
	}

	if err := service.Start(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	started, err := service.Load(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.State != StateRunning || started.StartedAt != 1100 || started.UpdatedAt != 1100 {
		t.Fatalf("started run = %+v", started)
	}

	if err := service.SetTotal(context.Background(), created.ID, -2); err != nil {
		t.Fatal(err)
	}
	totaled, err := service.Load(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if totaled.TotalCount != 0 || totaled.UpdatedAt != 1200 {
		t.Fatalf("totaled run = %+v", totaled)
	}

	detail := map[string]any{"success_count": 3}
	if err := service.Finish(context.Background(), FinishCommand{
		ID:            created.ID,
		FinishedCount: -1,
		Detail:        detail,
		LastError:     "  transient dial failure  ",
	}); err != nil {
		t.Fatal(err)
	}
	finished, err := service.Load(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.State != StateFinished || finished.Result != ResultSuccess || finished.ReasonCode != ReasonCompleted {
		t.Fatalf("finished run state/result = %+v", finished)
	}
	if finished.FinishedCount != 0 || finished.FinishedAt != 1300 || finished.UpdatedAt != 1300 {
		t.Fatalf("finished run counts/time = %+v", finished)
	}
	if finished.LastError != "transient dial failure" {
		t.Fatalf("last error = %q", finished.LastError)
	}
	if !reflect.DeepEqual(finished.Detail, detail) {
		t.Fatalf("detail = %#v, want %#v", finished.Detail, detail)
	}
}

func TestServiceClaimsNextQueuedRunAndListsByFilter(t *testing.T) {
	store := newMemoryRepository()
	service := NewService(store, func(prefix string) (string, error) {
		return prefix + "_one", nil
	}, millisClock(1000, 1100))

	created, err := service.Create(context.Background(), CreateCommand{
		RunType:       RunTypeSubscriptionRefresh,
		TriggerSource: TriggerScheduled,
		TargetID:      "sub_1",
		TargetLabel:   "Airport",
		TotalCount:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := service.ClaimNext(context.Background(), RunTypeSubscriptionRefresh)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected queued run to be claimed")
	}
	if claimed.ID != created.ID || claimed.State != StateRunning || claimed.StartedAt != 1100 {
		t.Fatalf("claimed run = %+v", claimed)
	}

	list, err := service.List(context.Background(), ListFilter{RunType: RunTypeSubscriptionRefresh, TargetID: "sub_1", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}
}

func TestServiceClaimsQueuedRunsOfTypeWithLimit(t *testing.T) {
	store := newMemoryRepository()
	service := NewService(store, func(prefix string) (string, error) {
		return prefix + "_unused", nil
	}, millisClock(1000, 1100, 1200))
	store.runs["run_one"] = Run{ID: "run_one", RunType: RunTypeNodeObservation, State: StateQueued}
	store.runs["run_two"] = Run{ID: "run_two", RunType: RunTypeNodeObservation, State: StateQueued}
	store.runs["run_other"] = Run{ID: "run_other", RunType: RunTypeProfileEvaluation, State: StateQueued}

	claimed, err := service.ClaimQueuedRunsOfType(context.Background(), RunTypeNodeObservation, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed len = %d, want 1", len(claimed))
	}
	if claimed[0].RunType != RunTypeNodeObservation || claimed[0].State != StateRunning {
		t.Fatalf("claimed run = %#v", claimed[0])
	}

	remaining, err := service.ClaimQueuedRunsOfType(context.Background(), RunTypeNodeObservation, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining claimed len = %d, want 1", len(remaining))
	}
	if store.runs["run_other"].State != StateQueued {
		t.Fatalf("other run state = %q, want queued", store.runs["run_other"].State)
	}
}

func TestServiceCancelsUnfinishedNodeObservationAggregateRuns(t *testing.T) {
	store := newMemoryRepository()
	service := NewService(store, func(prefix string) (string, error) {
		return prefix + "_unused", nil
	}, millisClock(5000, 6000))
	store.runs["run_all"] = Run{
		ID:            "run_all",
		RunType:       RunTypeNodeObservation,
		State:         StateQueued,
		FinishedCount: 1,
		Detail:        map[string]any{"target_scope": NodeObservationScopeAllNodes},
	}
	store.runs["run_due"] = Run{
		ID:      "run_due",
		RunType: RunTypeNodeObservation,
		State:   StateRunning,
		Detail:  map[string]any{"target_scope": NodeObservationScopeDueNodes},
	}
	store.runs["run_single"] = Run{
		ID:      "run_single",
		RunType: RunTypeNodeObservation,
		State:   StateQueued,
		Detail:  map[string]any{"target_scope": "single_node"},
	}
	store.runs["run_finished"] = Run{
		ID:      "run_finished",
		RunType: RunTypeNodeObservation,
		State:   StateFinished,
		Detail:  map[string]any{"target_scope": NodeObservationScopeAllNodes},
	}

	hasAggregate, err := service.HasUnfinishedNodeObservationAggregateRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hasAggregate {
		t.Fatal("expected unfinished aggregate node observation run")
	}

	if err := service.CancelUnfinishedNodeObservationAggregateRuns(context.Background(), "replaced_by_manual_run"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"run_all", "run_due"} {
		run := store.runs[id]
		if run.State != StateFinished || run.Result != ResultCancelled || run.ReasonCode != "replaced_by_manual_run" {
			t.Fatalf("%s = %#v", id, run)
		}
	}
	if store.runs["run_single"].State != StateQueued {
		t.Fatalf("single run state = %q, want queued", store.runs["run_single"].State)
	}
	if store.runs["run_finished"].Result != "" {
		t.Fatalf("finished aggregate should not be rewritten: %#v", store.runs["run_finished"])
	}
}

type memoryRepository struct {
	runs         map[string]Run
	repairResult DanglingProfileRepairResult
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{runs: map[string]Run{}}
}

func (r *memoryRepository) Insert(_ context.Context, run Run) error {
	r.runs[run.ID] = run
	return nil
}

func (r *memoryRepository) Load(_ context.Context, id string) (Run, error) {
	return r.runs[id], nil
}

func (r *memoryRepository) Start(_ context.Context, id string, nowMillis int64) error {
	run := r.runs[id]
	run.State = StateRunning
	if run.StartedAt == 0 {
		run.StartedAt = nowMillis
	}
	run.UpdatedAt = nowMillis
	r.runs[id] = run
	return nil
}

func (r *memoryRepository) SetTotal(_ context.Context, id string, totalCount int, nowMillis int64) error {
	run := r.runs[id]
	run.TotalCount = totalCount
	run.UpdatedAt = nowMillis
	r.runs[id] = run
	return nil
}

func (r *memoryRepository) Finish(_ context.Context, update FinishUpdate) error {
	run := r.runs[update.ID]
	run.State = StateFinished
	run.Result = update.Result
	run.ReasonCode = update.ReasonCode
	run.FinishedCount = update.FinishedCount
	run.Detail = update.Detail
	run.LastError = update.LastError
	run.FinishedAt = update.NowMillis
	run.UpdatedAt = update.NowMillis
	r.runs[update.ID] = run
	return nil
}

func (r *memoryRepository) ClaimNext(_ context.Context, runType string, nowMillis int64) (Run, bool, error) {
	for id, run := range r.runs {
		if run.RunType != runType || run.State != StateQueued {
			continue
		}
		run.State = StateRunning
		run.StartedAt = nowMillis
		run.UpdatedAt = nowMillis
		r.runs[id] = run
		return run, true, nil
	}
	return Run{}, false, nil
}

func (r *memoryRepository) List(_ context.Context, filter ListFilter) (ListResult, error) {
	items := make([]Run, 0, len(r.runs))
	for _, run := range r.runs {
		if filter.RunType != "" && run.RunType != filter.RunType {
			continue
		}
		if filter.TargetID != "" && run.TargetID != filter.TargetID {
			continue
		}
		if filter.State != "" && run.State != filter.State {
			continue
		}
		if filter.Result != "" && run.Result != filter.Result {
			continue
		}
		items = append(items, run)
	}
	return ListResult{Items: items, Total: len(items), Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *memoryRepository) ListProfileEvents(_ context.Context, profileID string, limit int) ([]Run, error) {
	var runs []Run
	for _, run := range r.runs {
		if run.TargetID == profileID && (run.RunType == "profile_evaluation" || run.RunType == "profile_switch") {
			runs = append(runs, run)
			if len(runs) >= limit {
				break
			}
		}
	}
	return runs, nil
}

func (r *memoryRepository) ListUnfinished(_ context.Context, runType string) ([]Run, error) {
	var runs []Run
	for _, run := range r.runs {
		if run.RunType == runType && run.State != StateFinished {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

func (r *memoryRepository) ListActive(_ context.Context) ([]Run, error) {
	var runs []Run
	for _, run := range r.runs {
		if run.State == StateQueued || run.State == StateRunning {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

func (r *memoryRepository) RepairDanglingProfilePaths(_ context.Context, _ int64) (DanglingProfileRepairResult, error) {
	return r.repairResult, nil
}

func millisClock(values ...int64) func() int64 {
	idx := 0
	return func() int64 {
		if idx >= len(values) {
			return values[len(values)-1]
		}
		value := values[idx]
		idx++
		return value
	}
}
