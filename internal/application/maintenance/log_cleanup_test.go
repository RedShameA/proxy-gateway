package maintenance

import (
	"context"
	"errors"
	"testing"
)

func TestBuildLogCleanupDetailPreservesCleanupFields(t *testing.T) {
	base := map[string]any{"existing": true}

	detail := BuildLogCleanupDetail(LogCleanupDetailInput{
		Detail:                             base,
		DeletedRequestLogs:                 3,
		DeletedMaintenanceRuns:             2,
		LogRetentionEnabled:                true,
		LogRetentionDays:                   14,
		MaintenanceHistoryRetentionEnabled: false,
		MaintenanceHistoryRetentionDays:    30,
	})

	if detail["deleted_request_logs"] != 3 || detail["deleted_maintenance_runs"] != 2 {
		t.Fatalf("deleted counts = %#v", detail)
	}
	if detail["log_retention_enabled"] != true || detail["log_retention_days"] != 14 {
		t.Fatalf("log retention fields = %#v", detail)
	}
	if detail["maintenance_history_retention_enabled"] != false || detail["maintenance_history_retention_days"] != 30 {
		t.Fatalf("maintenance retention fields = %#v", detail)
	}
	if base["deleted_request_logs"] != nil {
		t.Fatalf("base detail was mutated: %#v", base)
	}
}

func TestLogCleanupServiceExecutesRetentionCleanup(t *testing.T) {
	requestLogs := &fakeRequestLogRetentionRepository{deleted: 3}
	history := &fakeMaintenanceHistoryRetentionRepository{deleted: 2}
	service := LogCleanupService{
		RequestLogs:        requestLogs,
		MaintenanceHistory: history,
		NowMillis: func() int64 {
			return 10 * 86400 * 1000
		},
	}

	finish, err := service.Execute(context.Background(), Run{
		ID:     "run_cleanup",
		Detail: map[string]any{"existing": "value"},
	}, LogCleanupSettings{
		LogRetentionEnabled:                true,
		LogRetentionDays:                   7,
		MaintenanceHistoryRetentionEnabled: true,
		MaintenanceHistoryRetentionDays:    3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if finish.Result != ResultSuccess || finish.ReasonCode != ReasonCompleted {
		t.Fatalf("finish = %#v", finish)
	}
	if requestLogs.cutoff != 3*86400*1000 {
		t.Fatalf("request log cutoff = %d", requestLogs.cutoff)
	}
	if history.cutoff != 7*86400*1000 || history.keepRunID != "run_cleanup" {
		t.Fatalf("history cleanup = cutoff %d keep %q", history.cutoff, history.keepRunID)
	}
	if finish.Detail["deleted_request_logs"] != 3 || finish.Detail["deleted_maintenance_runs"] != 2 {
		t.Fatalf("detail = %#v", finish.Detail)
	}
}

func TestLogCleanupServiceReturnsFailureCommandOnRequestLogCleanupError(t *testing.T) {
	cleanupErr := errors.New("delete request logs")
	service := LogCleanupService{
		RequestLogs: &fakeRequestLogRetentionRepository{err: cleanupErr},
		NowMillis: func() int64 {
			return 10 * 86400 * 1000
		},
	}

	finish, err := service.Execute(context.Background(), Run{ID: "run_cleanup"}, LogCleanupSettings{
		LogRetentionEnabled: true,
		LogRetentionDays:    7,
	})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("Execute error = %v, want cleanupErr", err)
	}
	if finish.Result != ResultFailure || finish.ReasonCode != "request_log_cleanup_failed" || finish.LastError != cleanupErr.Error() {
		t.Fatalf("finish = %#v", finish)
	}
}

type fakeRequestLogRetentionRepository struct {
	deleted int64
	cutoff  int64
	err     error
}

func (r *fakeRequestLogRetentionRepository) DeleteBefore(_ context.Context, cutoffMillis int64) (int64, error) {
	r.cutoff = cutoffMillis
	return r.deleted, r.err
}

type fakeMaintenanceHistoryRetentionRepository struct {
	deleted   int64
	cutoff    int64
	keepRunID string
	err       error
}

func (r *fakeMaintenanceHistoryRetentionRepository) DeleteHistoryBefore(_ context.Context, cutoffMillis int64, keepRunID string) (int64, error) {
	r.cutoff = cutoffMillis
	r.keepRunID = keepRunID
	return r.deleted, r.err
}
