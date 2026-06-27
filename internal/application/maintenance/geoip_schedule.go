package maintenance

import (
	"time"

	appsettings "proxygateway/internal/application/settings"
)

type GeoIPScheduleStatus struct {
	LoadedAt  int64
	UpdatedAt int64
}

func ShouldQueueGeoIPUpdate(nowMillis int64, clock string, status GeoIPScheduleStatus) bool {
	scheduledAt, ok := GeoIPScheduledMillisForDay(nowMillis, clock)
	if !ok {
		return false
	}
	if status.LoadedAt > 0 && (nowMillis < scheduledAt || status.UpdatedAt >= scheduledAt) {
		return false
	}
	return true
}

func GeoIPScheduledMillisForDay(nowMillis int64, clock string) (int64, bool) {
	offset, ok := appsettings.ParseMaintenanceClock(clock)
	if !ok {
		return 0, false
	}
	now := time.UnixMilli(nowMillis).Local()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return startOfDay.Add(offset).UnixMilli(), true
}

func NextGeoIPScheduledMillis(nowMillis int64, clock string) (int64, bool) {
	offset, ok := appsettings.ParseMaintenanceClock(clock)
	if !ok {
		return 0, false
	}
	now := time.UnixMilli(nowMillis).Local()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	next := startOfDay.Add(offset)
	if !next.After(now) {
		next = startOfDay.AddDate(0, 0, 1).Add(offset)
	}
	return next.UnixMilli(), true
}
