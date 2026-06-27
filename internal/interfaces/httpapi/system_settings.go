package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	appsettings "proxygateway/internal/application/settings"
)

const (
	defaultRequestLogRetentionDays         = 10
	defaultMaintenanceHistoryRetentionDays = 7
	logRetentionPositiveMessage            = "请求日志保留天数必须大于 0"
	maintenanceHistoryPositiveMessage      = "维护历史保留天数必须大于 0"
	publicProxyEndpointInvalidMessage      = "代理访问地址格式无效"
	switchingToleranceNonNegativeMessage   = "切换容忍度不能为负数"
)

type SystemSettingsHandler struct {
	Auth        AuthFunc
	SystemRepo  appsettings.SystemRepository
	KVRepo      appsettings.KVRepository
	GeoIPStatus func() any
}

type systemEvaluationSettingsPatch struct {
	GlobalConcurrency                   *int `json:"global_concurrency"`
	DefaultMinEvaluationIntervalSeconds *int `json:"default_min_evaluation_interval_seconds"`
	SingleCandidateLimit                *int `json:"single_candidate_limit"`
	ChainCandidateLimit                 *int `json:"chain_candidate_limit"`
	ConnectTimeoutSeconds               *int `json:"connect_timeout_seconds"`
	ProbeTimeoutSeconds                 *int `json:"probe_timeout_seconds"`
}

type systemMaintenanceSettingsPatch struct {
	SubscriptionRefreshSeconds   *int    `json:"subscription_refresh_seconds"`
	NodeObservationSeconds       *int    `json:"node_observation_seconds"`
	ProfileEvaluationSeconds     *int    `json:"profile_evaluation_seconds"`
	ChainEvaluationSeconds       *int    `json:"chain_evaluation_seconds"`
	GeoIPUpdateTime              *string `json:"geoip_update_time"`
	EgressIPProbeURL             *string `json:"egress_ip_probe_url"`
	SubscriptionConcurrency      *int    `json:"subscription_concurrency"`
	NodeObservationConcurrency   *int    `json:"node_observation_concurrency"`
	ProfileEvaluationConcurrency *int    `json:"profile_evaluation_concurrency"`
	GeoIPConcurrency             *int    `json:"geoip_concurrency"`
}

type systemMaintenanceSchedulePatch struct {
	Key             string `json:"key"`
	Label           string `json:"label"`
	Enabled         *bool  `json:"enabled"`
	IntervalSeconds *int   `json:"interval_seconds"`
}

type systemSwitchingTolerancePatch struct {
	RelativeImprovementThreshold *float64 `json:"relative_improvement_threshold"`
	AbsoluteLatencyImprovementMS *int     `json:"absolute_latency_improvement_ms"`
}

func (h SystemSettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.get(w, r)
	case http.MethodPatch:
		h.patch(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h SystemSettingsHandler) get(w http.ResponseWriter, r *http.Request) {
	maint, _ := h.SystemRepo.LoadMaintenance(r.Context())
	maint = appsettings.NormalizeMaintenance(maint)
	eval, _ := h.SystemRepo.LoadEvaluation(r.Context())
	eval = appsettings.NormalizeEvaluation(eval)

	writeJSON(w, http.StatusOK, map[string]any{
		"public_proxy_endpoint":                 getKVSetting(r.Context(), h.KVRepo, "public_proxy_endpoint"),
		"log_retention_enabled":                 boolKVSettingDefaultTrue(getKVSetting(r.Context(), h.KVRepo, "log_retention_enabled")),
		"log_retention_days":                    systemSettingPositiveInt(getKVSetting(r.Context(), h.KVRepo, "log_retention_days"), defaultRequestLogRetentionDays),
		"maintenance_history_retention_enabled": boolKVSettingDefaultTrue(getKVSetting(r.Context(), h.KVRepo, "maintenance_history_retention_enabled")),
		"maintenance_history_retention_days":    systemSettingPositiveInt(getKVSetting(r.Context(), h.KVRepo, "maintenance_history_retention_days"), defaultMaintenanceHistoryRetentionDays),
		"maintenance":                           maint,
		"maintenance_schedules":                 maintenanceScheduleItems(maint),
		"switching_tolerance":                   loadSwitchingTolerance(r.Context(), h.KVRepo),
		"geoip":                                 h.geoIPStatus(),
		"evaluation":                            eval,
	})
}

func (h SystemSettingsHandler) patch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PublicEndpoint       *string                           `json:"public_proxy_endpoint"`
		LogRetentionOn       *bool                             `json:"log_retention_enabled"`
		LogRetentionDays     *int                              `json:"log_retention_days"`
		HistoryRetentionOn   *bool                             `json:"maintenance_history_retention_enabled"`
		HistoryRetentionDays *int                              `json:"maintenance_history_retention_days"`
		Maintenance          *systemMaintenanceSettingsPatch   `json:"maintenance"`
		Evaluation           *systemEvaluationSettingsPatch    `json:"evaluation"`
		Schedules            *[]systemMaintenanceSchedulePatch `json:"maintenance_schedules"`
		Tolerance            *systemSwitchingTolerancePatch    `json:"switching_tolerance"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if req.PublicEndpoint != nil {
		endpoint, err := normalizePublicProxyEndpoint(*req.PublicEndpoint)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.KVRepo.Set(r.Context(), "public_proxy_endpoint", endpoint); err != nil {
			writeError(w, http.StatusInternalServerError, "save public proxy endpoint")
			return
		}
	}
	if req.LogRetentionOn != nil {
		setBoolKVSetting(r.Context(), h.KVRepo, "log_retention_enabled", *req.LogRetentionOn)
	}
	if req.LogRetentionDays != nil {
		if *req.LogRetentionDays <= 0 {
			writeError(w, http.StatusBadRequest, logRetentionPositiveMessage)
			return
		}
		_ = h.KVRepo.Set(r.Context(), "log_retention_days", strconv.Itoa(*req.LogRetentionDays))
	}
	if req.HistoryRetentionOn != nil {
		setBoolKVSetting(r.Context(), h.KVRepo, "maintenance_history_retention_enabled", *req.HistoryRetentionOn)
	}
	if req.HistoryRetentionDays != nil {
		if *req.HistoryRetentionDays <= 0 {
			writeError(w, http.StatusBadRequest, maintenanceHistoryPositiveMessage)
			return
		}
		_ = h.KVRepo.Set(r.Context(), "maintenance_history_retention_days", strconv.Itoa(*req.HistoryRetentionDays))
	}
	if req.Evaluation != nil {
		settings, err := h.SystemRepo.LoadEvaluation(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load evaluation settings")
			return
		}
		settings = applyEvaluationSettingsPatch(settings, *req.Evaluation)
		settings = appsettings.NormalizeEvaluation(settings)
		if err := appsettings.ValidateEvaluation(settings); err != nil {
			writeError(w, http.StatusBadRequest, appsettings.EvaluationSettingsRangeError)
			return
		}
		if err := h.SystemRepo.SaveEvaluation(r.Context(), settings); err != nil {
			writeError(w, http.StatusInternalServerError, "save evaluation settings")
			return
		}
	}
	if req.Maintenance != nil || req.Schedules != nil {
		settings, err := h.SystemRepo.LoadMaintenance(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load maintenance settings")
			return
		}
		if req.Maintenance != nil {
			settings = applyMaintenanceSettingsPatch(settings, *req.Maintenance)
		}
		if req.Schedules != nil {
			settings = applyMaintenanceSchedulePatches(settings, *req.Schedules)
		}
		settings = appsettings.NormalizeMaintenance(settings)
		if err := appsettings.ValidateMaintenance(settings); err != nil {
			writeError(w, http.StatusBadRequest, maintenanceSettingsErrorMessage(err))
			return
		}
		if err := h.SystemRepo.SaveMaintenance(r.Context(), settings); err != nil {
			writeError(w, http.StatusInternalServerError, "save maintenance settings")
			return
		}
	}
	if req.Tolerance != nil {
		if err := saveSwitchingTolerance(r.Context(), h.KVRepo, *req.Tolerance); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	h.get(w, r)
}

func (h SystemSettingsHandler) geoIPStatus() any {
	if h.GeoIPStatus == nil {
		return nil
	}
	return h.GeoIPStatus()
}

func normalizePublicProxyEndpoint(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.Contains(value, "://") || strings.ContainsAny(value, "/?#@") || strings.ContainsAny(value, " \t\r\n") {
		return "", errors.New(publicProxyEndpointInvalidMessage)
	}
	if strings.HasPrefix(value, "[") {
		return validateBracketedProxyEndpoint(value)
	}
	if strings.Count(value, ":") > 1 {
		return "", errors.New(publicProxyEndpointInvalidMessage)
	}
	if strings.Contains(value, ":") {
		host, portText, err := net.SplitHostPort(value)
		if err != nil || !validProxyHost(host) || !validProxyPort(portText) {
			return "", errors.New(publicProxyEndpointInvalidMessage)
		}
		return value, nil
	}
	if !validProxyHost(value) {
		return "", errors.New(publicProxyEndpointInvalidMessage)
	}
	return value, nil
}

func validateBracketedProxyEndpoint(value string) (string, error) {
	if strings.HasSuffix(value, "]") {
		ipText := strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
		if ip := net.ParseIP(ipText); ip != nil && strings.Contains(ipText, ":") {
			return value, nil
		}
		return "", errors.New(publicProxyEndpointInvalidMessage)
	}
	host, portText, err := net.SplitHostPort(value)
	if err != nil || !validProxyPort(portText) {
		return "", errors.New(publicProxyEndpointInvalidMessage)
	}
	ipText := strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if ip := net.ParseIP(ipText); ip == nil || !strings.Contains(ipText, ":") {
		return "", errors.New(publicProxyEndpointInvalidMessage)
	}
	return value, nil
}

func validProxyHost(host string) bool {
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.To4() != nil
	}
	for _, ch := range host {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '.' {
			continue
		}
		return false
	}
	return !strings.Contains(host, "..") && !strings.HasPrefix(host, ".") && !strings.HasSuffix(host, ".")
}

func validProxyPort(portText string) bool {
	port, err := strconv.Atoi(portText)
	return err == nil && port > 0 && port <= 65535
}

func applyEvaluationSettingsPatch(settings appsettings.EvaluationSettings, patch systemEvaluationSettingsPatch) appsettings.EvaluationSettings {
	if patch.GlobalConcurrency != nil {
		settings.GlobalConcurrency = *patch.GlobalConcurrency
	}
	if patch.DefaultMinEvaluationIntervalSeconds != nil {
		settings.DefaultMinEvaluationIntervalSeconds = *patch.DefaultMinEvaluationIntervalSeconds
	}
	if patch.SingleCandidateLimit != nil {
		settings.SingleCandidateLimit = *patch.SingleCandidateLimit
	}
	if patch.ChainCandidateLimit != nil {
		settings.ChainCandidateLimit = *patch.ChainCandidateLimit
	}
	if patch.ConnectTimeoutSeconds != nil {
		settings.ConnectTimeoutSeconds = *patch.ConnectTimeoutSeconds
	}
	if patch.ProbeTimeoutSeconds != nil {
		settings.ProbeTimeoutSeconds = *patch.ProbeTimeoutSeconds
	}
	return settings
}

func applyMaintenanceSettingsPatch(settings appsettings.MaintenanceSettings, patch systemMaintenanceSettingsPatch) appsettings.MaintenanceSettings {
	if patch.SubscriptionRefreshSeconds != nil {
		settings.SubscriptionRefreshSeconds = *patch.SubscriptionRefreshSeconds
	}
	if patch.NodeObservationSeconds != nil {
		settings.NodeObservationSeconds = *patch.NodeObservationSeconds
	}
	if patch.ProfileEvaluationSeconds != nil {
		settings.ProfileEvaluationSeconds = *patch.ProfileEvaluationSeconds
	}
	if patch.ChainEvaluationSeconds != nil {
		settings.ChainEvaluationSeconds = *patch.ChainEvaluationSeconds
	}
	if patch.GeoIPUpdateTime != nil {
		settings.GeoIPUpdateTime = *patch.GeoIPUpdateTime
	}
	if patch.EgressIPProbeURL != nil {
		settings.EgressIPProbeURL = *patch.EgressIPProbeURL
	}
	if patch.SubscriptionConcurrency != nil {
		settings.SubscriptionConcurrency = *patch.SubscriptionConcurrency
	}
	if patch.NodeObservationConcurrency != nil {
		settings.NodeObservationConcurrency = *patch.NodeObservationConcurrency
	}
	if patch.ProfileEvaluationConcurrency != nil {
		settings.ProfileEvaluationConcurrency = *patch.ProfileEvaluationConcurrency
	}
	if patch.GeoIPConcurrency != nil {
		settings.GeoIPConcurrency = *patch.GeoIPConcurrency
	}
	return settings
}

func applyMaintenanceSchedulePatches(settings appsettings.MaintenanceSettings, patches []systemMaintenanceSchedulePatch) appsettings.MaintenanceSettings {
	for _, patch := range patches {
		enabled := true
		if patch.Enabled != nil {
			enabled = *patch.Enabled
		}
		interval := 0
		if patch.IntervalSeconds != nil {
			interval = *patch.IntervalSeconds
		}
		switch patch.Key {
		case "subscription_refresh":
			settings.SubscriptionRefreshSeconds = scheduleIntervalValue(enabled, interval, settings.SubscriptionRefreshSeconds, appsettings.DefaultSubscriptionRefreshSeconds)
		case "node_observation":
			settings.NodeObservationSeconds = scheduleIntervalValue(enabled, interval, settings.NodeObservationSeconds, appsettings.DefaultNodeObservationSeconds)
		case "profile_evaluation":
			settings.ProfileEvaluationSeconds = scheduleIntervalValue(enabled, interval, settings.ProfileEvaluationSeconds, appsettings.DefaultProfileEvaluationSeconds)
		case "geoip_update":
			if !enabled {
				settings.GeoIPUpdateTime = ""
			} else if settings.GeoIPUpdateTime == "" {
				settings.GeoIPUpdateTime = appsettings.DefaultGeoIPUpdateTime
			}
		}
	}
	return settings
}

func scheduleIntervalValue(enabled bool, requested, current, fallback int) int {
	if !enabled {
		return 0
	}
	if requested > 0 {
		return requested
	}
	if current > 0 {
		return current
	}
	return fallback
}

func maintenanceScheduleItems(s appsettings.MaintenanceSettings) []map[string]any {
	return []map[string]any{
		{"key": "subscription_refresh", "label": "订阅刷新", "enabled": s.SubscriptionRefreshSeconds > 0, "interval_seconds": s.SubscriptionRefreshSeconds},
		{"key": "node_observation", "label": "节点观测", "enabled": s.NodeObservationSeconds > 0, "interval_seconds": s.NodeObservationSeconds},
		{"key": "profile_evaluation", "label": "策略评估", "enabled": s.ProfileEvaluationSeconds > 0, "interval_seconds": s.ProfileEvaluationSeconds},
		{"key": "geoip_update", "label": "GeoIP 更新", "enabled": s.GeoIPUpdateTime != "", "interval_seconds": 86400},
	}
}

func loadSwitchingTolerance(ctx context.Context, repo appsettings.KVRepository) map[string]any {
	relative := 0.2
	if value := getKVSetting(ctx, repo, "switching_tolerance.relative_improvement_threshold"); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed >= 0 {
			relative = parsed
		}
	}
	absolute := 100
	if value := getKVSetting(ctx, repo, "switching_tolerance.absolute_latency_improvement_ms"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 {
			absolute = parsed
		}
	}
	return map[string]any{
		"relative_improvement_threshold":  relative,
		"absolute_latency_improvement_ms": absolute,
	}
}

func saveSwitchingTolerance(ctx context.Context, repo appsettings.KVRepository, patch systemSwitchingTolerancePatch) error {
	if patch.RelativeImprovementThreshold != nil {
		if *patch.RelativeImprovementThreshold < 0 {
			return errors.New(switchingToleranceNonNegativeMessage)
		}
		if err := repo.Set(ctx, "switching_tolerance.relative_improvement_threshold", strconv.FormatFloat(*patch.RelativeImprovementThreshold, 'f', -1, 64)); err != nil {
			return err
		}
	}
	if patch.AbsoluteLatencyImprovementMS != nil {
		if *patch.AbsoluteLatencyImprovementMS < 0 {
			return errors.New(switchingToleranceNonNegativeMessage)
		}
		if err := repo.Set(ctx, "switching_tolerance.absolute_latency_improvement_ms", strconv.Itoa(*patch.AbsoluteLatencyImprovementMS)); err != nil {
			return err
		}
	}
	return nil
}

func getKVSetting(ctx context.Context, repo appsettings.KVRepository, key string) string {
	value, ok, err := repo.Get(ctx, key)
	if err != nil || !ok {
		return ""
	}
	return value
}

func setBoolKVSetting(ctx context.Context, repo appsettings.KVRepository, key string, value bool) {
	if value {
		_ = repo.Set(ctx, key, "true")
		return
	}
	_ = repo.Set(ctx, key, "false")
}

func boolKVSettingDefaultTrue(value string) bool {
	return value != "false"
}

func systemSettingPositiveInt(value string, fallback int) int {
	if parsed := parseInt(value); parsed > 0 {
		return parsed
	}
	return fallback
}
