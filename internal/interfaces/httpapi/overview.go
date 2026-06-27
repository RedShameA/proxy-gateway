package httpapi

import (
	"net/http"

	appmaintenance "proxygateway/internal/application/maintenance"
	appoverview "proxygateway/internal/application/overview"
	appproxy "proxygateway/internal/application/proxy"
)

type OverviewHandler struct {
	Auth           AuthFunc
	OverviewRepo   appoverview.Repository
	RequestLogRepo appproxy.RequestLogRepository
	Maintenance    MaintenanceRunService
	AccessProfiles func() any
	GeoIPStatus    func() any
	Now            func() int64
}

func (h OverviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var counts struct {
		Subscriptions int `json:"subscriptions"`
		Nodes         int `json:"nodes"`
		UsableNodes   int `json:"usable_nodes"`
		Profiles      int `json:"access_profiles"`
		Credentials   int `json:"proxy_credentials"`
		Requests24h   int `json:"requests_24h"`
		Failed24h     int `json:"failed_requests_24h"`
	}
	if resourceCounts, err := h.OverviewRepo.LoadResourceCounts(r.Context()); err == nil {
		counts.Subscriptions = resourceCounts.Subscriptions
		counts.Nodes = resourceCounts.Nodes
		counts.UsableNodes = resourceCounts.UsableNodes
		counts.Profiles = resourceCounts.Profiles
		counts.Credentials = resourceCounts.Credentials
	}
	requestLogCutoff := h.nowMillis() - 86400*1000
	if requestLogCounts, err := h.RequestLogRepo.CountSince(r.Context(), requestLogCutoff); err == nil {
		counts.Requests24h = requestLogCounts.Total
		counts.Failed24h = requestLogCounts.Failed
	}

	stateCounts := map[string]int{
		"pending": 0, "running": 0, "waiting_observation": 0, "ready": 0, "degraded": 0,
		"no_candidate": 0, "failed": 0, "invalid_config": 0,
	}
	if loadedStateCounts, err := h.OverviewRepo.LoadProfileStateCounts(r.Context()); err == nil {
		for state, count := range loadedStateCounts {
			stateCounts[state] = count
		}
	}

	failures := []map[string]any{}
	if recentFailures, err := h.RequestLogRepo.ListRecentFailures(r.Context(), 10); err == nil {
		nowMillis := h.nowMillis()
		for _, failure := range recentFailures {
			failures = append(failures, appproxy.RequestLogEntryToMap(failure, nowMillis))
		}
	}

	maintenanceRuns := []map[string]any{}
	if list, err := h.Maintenance.List(r.Context(), appmaintenance.ListFilter{Page: 1, PageSize: 10}); err == nil {
		for _, item := range list.Items {
			maintenanceRuns = append(maintenanceRuns, appmaintenance.RunToMap(item))
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"resource_counts":      counts,
		"profile_state_counts": stateCounts,
		"access_profiles":      h.accessProfiles(),
		"recent_failures":      failures,
		"maintenance_runs":     maintenanceRuns,
		"geoip_status":         h.geoIPStatus(),
	})
}

func (h OverviewHandler) nowMillis() int64 {
	if h.Now != nil {
		return h.Now()
	}
	return 0
}

func (h OverviewHandler) accessProfiles() any {
	if h.AccessProfiles == nil {
		return []any{}
	}
	return h.AccessProfiles()
}

func (h OverviewHandler) geoIPStatus() any {
	if h.GeoIPStatus == nil {
		return nil
	}
	return h.GeoIPStatus()
}
