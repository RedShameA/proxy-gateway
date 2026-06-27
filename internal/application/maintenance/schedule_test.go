package maintenance

import (
	"testing"
	"time"
)

func TestDueProfileEvaluationTargetsUsesProfileChainAndCustomIntervals(t *testing.T) {
	targets := []ProfileEvaluationScheduleTarget{
		{ID: "disabled", AutoEvaluationEnabled: false},
		{ID: "fastest_due", ProfileType: "fastest", AutoEvaluationEnabled: true, LastEvaluatedAt: 1000},
		{ID: "fastest_recent", ProfileType: "fastest", AutoEvaluationEnabled: true, LastEvaluatedAt: 9500},
		{ID: "chain_due", ProfileType: "chain", AutoEvaluationEnabled: true, LastEvaluatedAt: 1000},
		{ID: "chain_recent", ProfileType: "chain", AutoEvaluationEnabled: true, LastEvaluatedAt: 8000},
		{ID: "custom_due", ProfileType: "fastest", AutoEvaluationEnabled: true, AutoEvaluationIntervalSeconds: 2, LastEvaluatedAt: 7000},
	}

	due := DueProfileEvaluationTargets(targets, 10000, 5, 3)

	if got := idsFromProfileTargets(due); len(got) != 3 || got[0] != "fastest_due" || got[1] != "chain_due" || got[2] != "custom_due" {
		t.Fatalf("due ids = %#v", got)
	}
}

func TestDueSubscriptionRefreshTargetsUsesDefaultAndCustomIntervals(t *testing.T) {
	targets := []SubscriptionRefreshScheduleTarget{
		{ID: "disabled", AutoRefreshEnabled: false},
		{ID: "never_refreshed", AutoRefreshEnabled: true},
		{ID: "default_recent", AutoRefreshEnabled: true, UpdatedAt: 9500},
		{ID: "default_due", AutoRefreshEnabled: true, UpdatedAt: 1000},
		{ID: "custom_due", AutoRefreshEnabled: true, AutoRefreshIntervalSeconds: 2, UpdatedAt: 7000},
	}

	due := DueSubscriptionRefreshTargets(targets, 10000, 5)

	if got := idsFromSubscriptionTargets(due); len(got) != 3 || got[0] != "never_refreshed" || got[1] != "default_due" || got[2] != "custom_due" {
		t.Fatalf("due ids = %#v", got)
	}
}

func TestGeoIPScheduleQueuesAfterDailyClockWhenNotUpdated(t *testing.T) {
	now := time.Date(2026, 6, 27, 8, 0, 0, 0, time.Local).UnixMilli()
	status := GeoIPScheduleStatus{
		LoadedAt:  time.Date(2026, 6, 26, 8, 0, 0, 0, time.Local).UnixMilli(),
		UpdatedAt: time.Date(2026, 6, 26, 8, 0, 0, 0, time.Local).UnixMilli(),
	}

	if !ShouldQueueGeoIPUpdate(now, "07:00", status) {
		t.Fatal("expected geoip update to be queued after scheduled clock")
	}
}

func TestGeoIPScheduleSkipsBeforeClockAndAfterAlreadyUpdated(t *testing.T) {
	beforeClock := time.Date(2026, 6, 27, 6, 30, 0, 0, time.Local).UnixMilli()
	loaded := time.Date(2026, 6, 26, 8, 0, 0, 0, time.Local).UnixMilli()
	if ShouldQueueGeoIPUpdate(beforeClock, "07:00", GeoIPScheduleStatus{LoadedAt: loaded, UpdatedAt: loaded}) {
		t.Fatal("expected geoip update to wait until scheduled clock")
	}

	afterClock := time.Date(2026, 6, 27, 8, 0, 0, 0, time.Local).UnixMilli()
	updatedToday := time.Date(2026, 6, 27, 7, 30, 0, 0, time.Local).UnixMilli()
	if ShouldQueueGeoIPUpdate(afterClock, "07:00", GeoIPScheduleStatus{LoadedAt: loaded, UpdatedAt: updatedToday}) {
		t.Fatal("expected geoip update to skip after already updating today")
	}
}

func TestNextGeoIPScheduledMillisReturnsFutureSchedule(t *testing.T) {
	now := time.Date(2026, 6, 27, 8, 0, 0, 0, time.Local).UnixMilli()

	next, ok := NextGeoIPScheduledMillis(now, "07:00")
	if !ok {
		t.Fatal("expected next geoip schedule to parse")
	}
	want := time.Date(2026, 6, 28, 7, 0, 0, 0, time.Local).UnixMilli()
	if next != want {
		t.Fatalf("next = %d, want %d", next, want)
	}
}

func idsFromProfileTargets(targets []ProfileEvaluationScheduleTarget) []string {
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ID)
	}
	return ids
}

func idsFromSubscriptionTargets(targets []SubscriptionRefreshScheduleTarget) []string {
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ID)
	}
	return ids
}
