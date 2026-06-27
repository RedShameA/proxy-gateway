package settings

import "context"

type KVRepository interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string) error
}

type EvaluationSettings struct {
	GlobalConcurrency                   int `json:"global_concurrency"`
	DefaultMinEvaluationIntervalSeconds int `json:"default_min_evaluation_interval_seconds"`
	SingleCandidateLimit                int `json:"single_candidate_limit"`
	ChainCandidateLimit                 int `json:"chain_candidate_limit"`
	ConnectTimeoutSeconds               int `json:"connect_timeout_seconds"`
	ProbeTimeoutSeconds                 int `json:"probe_timeout_seconds"`
}

type MaintenanceSettings struct {
	SubscriptionRefreshSeconds   int    `json:"subscription_refresh_seconds"`
	NodeObservationSeconds       int    `json:"node_observation_seconds"`
	ProfileEvaluationSeconds     int    `json:"profile_evaluation_seconds"`
	ChainEvaluationSeconds       int    `json:"chain_evaluation_seconds"`
	GeoIPUpdateTime              string `json:"geoip_update_time"`
	EgressIPProbeURL             string `json:"egress_ip_probe_url"`
	SubscriptionConcurrency      int    `json:"subscription_concurrency"`
	NodeObservationConcurrency   int    `json:"node_observation_concurrency"`
	ProfileEvaluationConcurrency int    `json:"profile_evaluation_concurrency"`
	GeoIPConcurrency             int    `json:"geoip_concurrency"`
}

type SystemRepository interface {
	LoadEvaluation(ctx context.Context) (EvaluationSettings, error)
	SaveEvaluation(ctx context.Context, settings EvaluationSettings) error
	LoadMaintenance(ctx context.Context) (MaintenanceSettings, error)
	SaveMaintenance(ctx context.Context, settings MaintenanceSettings) error
}
