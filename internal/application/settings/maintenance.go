package settings

import (
	"errors"
	"strings"
	"time"
)

const (
	DefaultEgressIPProbeURL              = "https://cloudflare.com/cdn-cgi/trace"
	MaintenanceIntervalsNonNegativeError = "维护调度间隔不能为负数"
	GeoIPTimeFormatError                 = "GeoIP 更新时间必须使用 HH:MM 格式"
	MaintenanceConcurrencyPositiveError  = "维护并发数必须大于 0"
	DefaultSubscriptionRefreshSeconds    = 21600
	DefaultNodeObservationSeconds        = 1800
	DefaultProfileEvaluationSeconds      = 300
	DefaultChainEvaluationSeconds        = 900
	DefaultSubscriptionConcurrency       = 1
	DefaultNodeObservationConcurrency    = 8
	DefaultProfileEvaluationConcurrency  = 2
	DefaultGeoIPConcurrency              = 1
	DefaultGeoIPUpdateTime               = "07:00"
)

var (
	ErrMaintenanceIntervalsNonNegative = errors.New(MaintenanceIntervalsNonNegativeError)
	ErrGeoIPTimeFormat                 = errors.New(GeoIPTimeFormatError)
	ErrMaintenanceConcurrencyPositive  = errors.New(MaintenanceConcurrencyPositiveError)
)

func NormalizeMaintenance(settings MaintenanceSettings) MaintenanceSettings {
	if settings.EgressIPProbeURL == "" {
		settings.EgressIPProbeURL = DefaultEgressIPProbeURL
	}
	if settings.SubscriptionConcurrency <= 0 {
		settings.SubscriptionConcurrency = DefaultSubscriptionConcurrency
	}
	if settings.NodeObservationConcurrency <= 0 {
		settings.NodeObservationConcurrency = DefaultNodeObservationConcurrency
	}
	if settings.ProfileEvaluationConcurrency <= 0 {
		settings.ProfileEvaluationConcurrency = DefaultProfileEvaluationConcurrency
	}
	if settings.GeoIPConcurrency <= 0 {
		settings.GeoIPConcurrency = DefaultGeoIPConcurrency
	}
	return settings
}

func ValidateMaintenance(settings MaintenanceSettings) error {
	if settings.SubscriptionRefreshSeconds < 0 ||
		settings.NodeObservationSeconds < 0 ||
		settings.ProfileEvaluationSeconds < 0 ||
		settings.ChainEvaluationSeconds < 0 {
		return ErrMaintenanceIntervalsNonNegative
	}
	if strings.TrimSpace(settings.GeoIPUpdateTime) != "" {
		if _, ok := ParseMaintenanceClock(settings.GeoIPUpdateTime); !ok {
			return ErrGeoIPTimeFormat
		}
	}
	if settings.SubscriptionConcurrency <= 0 ||
		settings.NodeObservationConcurrency <= 0 ||
		settings.ProfileEvaluationConcurrency <= 0 ||
		settings.GeoIPConcurrency <= 0 {
		return ErrMaintenanceConcurrencyPositive
	}
	return nil
}

func ParseMaintenanceClock(value string) (time.Duration, bool) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute, true
}
