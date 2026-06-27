package maintenance

import (
	"context"
	"testing"
)

func TestStartupCleanupServiceCancelsActiveRunsAndRecordsSummary(t *testing.T) {
	store := newMemoryRepository()
	store.runs["run_queued"] = Run{
		ID:            "run_queued",
		RunType:       RunTypeProfileEvaluation,
		State:         StateQueued,
		FinishedCount: 1,
		Detail:        map[string]any{"profile_id": "profile_1"},
		LastError:     "old error",
	}
	store.runs["run_running"] = Run{
		ID:      "run_running",
		RunType: RunTypeNodeObservation,
		State:   StateRunning,
	}
	store.runs["run_finished"] = Run{
		ID:     "run_finished",
		State:  StateFinished,
		Result: ResultSuccess,
	}
	store.repairResult = DanglingProfileRepairResult{
		RepairedCount: 2,
		InvalidCount:  1,
		EvaluationRefs: []ProfileEvaluationRef{{
			ID:   "profile_1",
			Name: "Profile",
		}},
	}
	service := NewService(store, func(prefix string) (string, error) {
		if prefix != "run" {
			t.Fatalf("prefix = %q, want run", prefix)
		}
		return "run_startup", nil
	}, millisClock(1000, 1100, 1200, 1300))

	result, err := StartupCleanupService{Runs: service}.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.CancelledCount != 2 || result.RepairedCount != 2 || result.InvalidCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(result.EvaluationRefs) != 1 || result.EvaluationRefs[0].ID != "profile_1" {
		t.Fatalf("evaluation refs = %#v", result.EvaluationRefs)
	}
	if store.runs["run_queued"].Result != ResultCancelled || store.runs["run_queued"].ReasonCode != "expired_after_restart" {
		t.Fatalf("queued run after cleanup = %#v", store.runs["run_queued"])
	}
	if store.runs["run_running"].Result != ResultCancelled || store.runs["run_running"].ReasonCode != "expired_after_restart" {
		t.Fatalf("running run after cleanup = %#v", store.runs["run_running"])
	}
	startup := store.runs["run_startup"]
	if startup.RunType != RunTypeStartupCleanup || startup.State != StateFinished || startup.Result != ResultSuccess {
		t.Fatalf("startup run = %#v", startup)
	}
	if startup.FinishedCount != 2 || startup.Detail["cancelled_count"] != 2 || startup.Detail["repaired_profile_count"] != 2 || startup.Detail["invalid_profile_count"] != 1 {
		t.Fatalf("startup detail = %#v", startup)
	}
	if store.runs["run_finished"].Result != ResultSuccess {
		t.Fatalf("finished run should not be rewritten: %#v", store.runs["run_finished"])
	}
}
