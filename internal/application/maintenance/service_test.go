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
		RunType:       "profile_evaluation",
		TriggerSource: "manual",
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
		RunType:       "subscription_refresh",
		TriggerSource: "scheduled",
		TargetID:      "sub_1",
		TargetLabel:   "Airport",
		TotalCount:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := service.ClaimNext(context.Background(), "subscription_refresh")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected queued run to be claimed")
	}
	if claimed.ID != created.ID || claimed.State != StateRunning || claimed.StartedAt != 1100 {
		t.Fatalf("claimed run = %+v", claimed)
	}

	list, err := service.List(context.Background(), ListFilter{RunType: "subscription_refresh", TargetID: "sub_1", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}
}

type memoryRepository struct {
	runs map[string]Run
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
