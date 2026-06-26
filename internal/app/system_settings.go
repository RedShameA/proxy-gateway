package app

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
)

const (
	defaultRequestLogRetentionDays         = 10
	defaultMaintenanceHistoryRetentionDays = 7
)

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

func (g *Gateway) handleSystemSettings(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		g.getSystemSettings(w)
	case http.MethodPatch:
		g.patchSystemSettings(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (g *Gateway) getSystemSettings(w http.ResponseWriter) {
	// Load maintenance settings
	maint, _ := g.loadMaintenanceSettings()

	// Load evaluation settings
	eval, _ := g.loadEvaluationSettings()

	// Get GeoIP status
	geoip := g.geoIPStatus()

	// Get public_proxy_endpoint from kv store
	publicEndpoint := g.getKVSetting("public_proxy_endpoint")
	logRetentionEnabled := boolKVSettingDefaultTrue(g.getKVSetting("log_retention_enabled"))
	logRetentionDays := defaultRequestLogRetentionDays
	if v := g.getKVSetting("log_retention_days"); v != "" {
		if d := parseInt(v); d > 0 {
			logRetentionDays = d
		}
	}
	maintenanceHistoryRetentionEnabled := boolKVSettingDefaultTrue(g.getKVSetting("maintenance_history_retention_enabled"))
	maintenanceHistoryRetentionDays := systemSettingPositiveInt(g.getKVSetting("maintenance_history_retention_days"), defaultMaintenanceHistoryRetentionDays)

	writeJSON(w, http.StatusOK, map[string]any{
		"public_proxy_endpoint":                 publicEndpoint,
		"log_retention_enabled":                 logRetentionEnabled,
		"log_retention_days":                    logRetentionDays,
		"maintenance_history_retention_enabled": maintenanceHistoryRetentionEnabled,
		"maintenance_history_retention_days":    maintenanceHistoryRetentionDays,
		"maintenance":                           maint,
		"maintenance_schedules":                 maintenanceScheduleItems(maint),
		"switching_tolerance":                   g.loadSwitchingTolerance(),
		"geoip":                                 geoip,
		"evaluation":                            eval,
	})
}

func (g *Gateway) patchSystemSettings(w http.ResponseWriter, r *http.Request) {
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
		if err := g.setKVSetting("public_proxy_endpoint", endpoint); err != nil {
			writeError(w, http.StatusInternalServerError, "save public proxy endpoint")
			return
		}
	}
	if req.LogRetentionOn != nil {
		if *req.LogRetentionOn {
			g.setKVSetting("log_retention_enabled", "true")
		} else {
			g.setKVSetting("log_retention_enabled", "false")
		}
	}
	if req.LogRetentionDays != nil {
		if *req.LogRetentionDays <= 0 {
			writeError(w, http.StatusBadRequest, validationLogRetentionPositive)
			return
		}
		g.setKVSetting("log_retention_days", strconv.Itoa(*req.LogRetentionDays))
	}
	if req.HistoryRetentionOn != nil {
		if *req.HistoryRetentionOn {
			g.setKVSetting("maintenance_history_retention_enabled", "true")
		} else {
			g.setKVSetting("maintenance_history_retention_enabled", "false")
		}
	}
	if req.HistoryRetentionDays != nil {
		if *req.HistoryRetentionDays <= 0 {
			writeError(w, http.StatusBadRequest, validationMaintenanceHistoryPositive)
			return
		}
		g.setKVSetting("maintenance_history_retention_days", strconv.Itoa(*req.HistoryRetentionDays))
	}
	if req.Evaluation != nil {
		settings, err := g.loadEvaluationSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load evaluation settings")
			return
		}
		settings = applyEvaluationSettingsPatch(settings, *req.Evaluation)
		settings = normalizeEvaluationSettings(settings)
		if err := validateEvaluationSettings(settings); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := g.saveEvaluationSettings(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "save evaluation settings")
			return
		}
	}
	if req.Maintenance != nil || req.Schedules != nil {
		settings, err := g.loadMaintenanceSettings()
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
		settings = normalizeMaintenanceSettings(settings)
		if err := validateMaintenanceSettings(settings); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := g.saveMaintenanceSettings(settings); err != nil {
			writeError(w, http.StatusInternalServerError, "save maintenance settings")
			return
		}
	}
	if req.Tolerance != nil {
		if err := g.saveSwitchingTolerance(*req.Tolerance); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Return updated settings
	g.getSystemSettings(w)
}

func boolKVSettingDefaultTrue(value string) bool {
	return value != "false"
}

func normalizePublicProxyEndpoint(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.Contains(value, "://") || strings.ContainsAny(value, "/?#@") || strings.ContainsAny(value, " \t\r\n") {
		return "", errors.New(validationPublicProxyEndpointInvalid)
	}
	if strings.HasPrefix(value, "[") {
		return validateBracketedProxyEndpoint(value)
	}
	if strings.Count(value, ":") > 1 {
		return "", errors.New(validationPublicProxyEndpointInvalid)
	}
	if strings.Contains(value, ":") {
		host, portText, err := net.SplitHostPort(value)
		if err != nil || !validProxyHost(host) || !validProxyPort(portText) {
			return "", errors.New(validationPublicProxyEndpointInvalid)
		}
		return value, nil
	}
	if !validProxyHost(value) {
		return "", errors.New(validationPublicProxyEndpointInvalid)
	}
	return value, nil
}

func validateBracketedProxyEndpoint(value string) (string, error) {
	if strings.HasSuffix(value, "]") {
		ipText := strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
		if ip := net.ParseIP(ipText); ip != nil && strings.Contains(ipText, ":") {
			return value, nil
		}
		return "", errors.New(validationPublicProxyEndpointInvalid)
	}
	host, portText, err := net.SplitHostPort(value)
	if err != nil || !validProxyPort(portText) {
		return "", errors.New(validationPublicProxyEndpointInvalid)
	}
	ipText := strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if ip := net.ParseIP(ipText); ip == nil || !strings.Contains(ipText, ":") {
		return "", errors.New(validationPublicProxyEndpointInvalid)
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

func applyEvaluationSettingsPatch(settings evaluationSettings, patch systemEvaluationSettingsPatch) evaluationSettings {
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

func applyMaintenanceSettingsPatch(settings maintenanceSettings, patch systemMaintenanceSettingsPatch) maintenanceSettings {
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

func applyMaintenanceSchedulePatches(settings maintenanceSettings, patches []systemMaintenanceSchedulePatch) maintenanceSettings {
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
			settings.SubscriptionRefreshSeconds = scheduleIntervalValue(enabled, interval, settings.SubscriptionRefreshSeconds, 21600)
		case "node_observation":
			settings.NodeObservationSeconds = scheduleIntervalValue(enabled, interval, settings.NodeObservationSeconds, 1800)
		case "profile_evaluation":
			settings.ProfileEvaluationSeconds = scheduleIntervalValue(enabled, interval, settings.ProfileEvaluationSeconds, 300)
		case "geoip_update":
			if !enabled {
				settings.GeoIPUpdateTime = ""
			} else if settings.GeoIPUpdateTime == "" {
				settings.GeoIPUpdateTime = "07:00"
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

func (g *Gateway) getKVSetting(key string) string {
	var value string
	err := g.db.QueryRow(`SELECT value FROM kv_settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

func (g *Gateway) setKVSetting(key, value string) error {
	_, err := g.db.Exec(`INSERT OR REPLACE INTO kv_settings (key, value) VALUES (?, ?)`, key, value)
	return err
}

func maintenanceScheduleItems(s maintenanceSettings) []map[string]any {
	return []map[string]any{
		{"key": "subscription_refresh", "label": "订阅刷新", "enabled": s.SubscriptionRefreshSeconds > 0, "interval_seconds": s.SubscriptionRefreshSeconds},
		{"key": "node_observation", "label": "节点观测", "enabled": s.NodeObservationSeconds > 0, "interval_seconds": s.NodeObservationSeconds},
		{"key": "profile_evaluation", "label": "策略评估", "enabled": s.ProfileEvaluationSeconds > 0, "interval_seconds": s.ProfileEvaluationSeconds},
		{"key": "geoip_update", "label": "GeoIP 更新", "enabled": s.GeoIPUpdateTime != "", "interval_seconds": 86400},
	}
}

func (g *Gateway) loadSwitchingTolerance() map[string]any {
	relative := 0.2
	if value := g.getKVSetting("switching_tolerance.relative_improvement_threshold"); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed >= 0 {
			relative = parsed
		}
	}
	absolute := 100
	if value := g.getKVSetting("switching_tolerance.absolute_latency_improvement_ms"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= 0 {
			absolute = parsed
		}
	}
	return map[string]any{
		"relative_improvement_threshold":  relative,
		"absolute_latency_improvement_ms": absolute,
	}
}

func (g *Gateway) saveSwitchingTolerance(patch systemSwitchingTolerancePatch) error {
	if patch.RelativeImprovementThreshold != nil {
		if *patch.RelativeImprovementThreshold < 0 {
			return errInvalidSwitchingTolerance()
		}
		if err := g.setKVSetting("switching_tolerance.relative_improvement_threshold", strconv.FormatFloat(*patch.RelativeImprovementThreshold, 'f', -1, 64)); err != nil {
			return err
		}
	}
	if patch.AbsoluteLatencyImprovementMS != nil {
		if *patch.AbsoluteLatencyImprovementMS < 0 {
			return errInvalidSwitchingTolerance()
		}
		if err := g.setKVSetting("switching_tolerance.absolute_latency_improvement_ms", strconv.Itoa(*patch.AbsoluteLatencyImprovementMS)); err != nil {
			return err
		}
	}
	return nil
}

func errInvalidSwitchingTolerance() error {
	return &settingsValidationError{message: validationSwitchingToleranceNonNegative}
}

type settingsValidationError struct {
	message string
}

func (err *settingsValidationError) Error() string {
	return err.message
}
