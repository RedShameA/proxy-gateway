/* ===== Core API types from PRD 0044 ===== */

export type UnixMillis = number;

export type AccessProfileType = 'fixed_node' | 'fastest' | 'random' | 'chain';
export type ProfileState = 'pending' | 'running' | 'waiting_observation' | 'ready' | 'degraded' | 'no_candidate' | 'failed' | 'invalid_config';
export type ChainEvaluationMode = 'chain_link' | 'end_to_end';
export type LatencyKind = 'end_to_end' | 'chain_link';

export type EgressCountryValue = string;

export type EgressCountryDisplay = {
  value: EgressCountryValue;
  iso_code: string | null;
  name_zh: string;
  is_unknown: boolean;
};

export type NodePathSummary = {
  id: string;
  name: string;
  protocol: string;
  server: string;
  server_port: number;
  egress_ip: string | null;
  egress_country: EgressCountryDisplay;
  observation_latency_ms: number | null;
  last_observed_at: UnixMillis | null;
};

export type ProxyPathSummary =
  | { path_type: 'single'; node: NodePathSummary; latency_ms: number | null; latency_kind: LatencyKind | null; evaluated_at: UnixMillis | null; }
  | { path_type: 'chain'; front_node: NodePathSummary; exit_node: NodePathSummary; final_egress_country: EgressCountryDisplay; chain_evaluation_mode: ChainEvaluationMode; latency_ms: number | null; latency_kind: LatencyKind | null; evaluated_at: UnixMillis | null; };

export type CandidateFilter = {
  source_mode: 'all' | 'manual' | 'subscription' | 'selected_sources';
  source_ids: string[];
  protocols: string[];
  name_include: string;
  name_exclude: string;
  egress_country_mode: 'include' | 'exclude';
  egress_countries: EgressCountryValue[];
};

export type SwitchingTolerance = {
  relative_improvement_threshold: number;
  absolute_latency_improvement_ms: number;
};

export type EvaluationSchedulePolicy = {
  mode: 'inherit' | 'custom' | 'disabled';
  interval_seconds: number | null;
};

export type MaintenanceRunType =
  | 'node_observation'
  | 'profile_evaluation'
  | 'profile_switch'
  | 'subscription_refresh'
  | 'geoip_update'
  | 'log_cleanup'
  | 'startup_cleanup';

export type MaintenanceRunState = 'queued' | 'running' | 'finished';
export type MaintenanceRunResult = '' | 'success' | 'warning' | 'failure' | 'skipped' | 'cancelled';

export type MaintenanceRunSummary = {
  id: string;
  run_type: MaintenanceRunType;
  trigger_source: string;
  target_id: string;
  target_label: string;
  state: MaintenanceRunState;
  result: MaintenanceRunResult;
  reason_code: string;
  total_count: number;
  finished_count: number;
  detail: Record<string, unknown>;
  last_error: string;
  created_at: UnixMillis;
  started_at: UnixMillis | null;
  finished_at: UnixMillis | null;
  updated_at: UnixMillis;
};

export type AccessProfileSummary = {
  id: string;
  name: string;
  type: AccessProfileType;
  state: ProfileState;
  profile_identifier: string;
  current_path: ProxyPathSummary | null;
  proxy_credentials_count: number;
  enabled_proxy_credentials_count: number;
  node_sticky_enabled: boolean;
  last_evaluated_at: UnixMillis | null;
  last_error: string;
  switch_reason: string;
};

export type AccessProfileDetail = AccessProfileSummary & {
  fixed_node_id: string | null;
  exit_node_ids: string[];
  chain_evaluation_mode: ChainEvaluationMode | null;
  test_url: string;
  candidate_filter: CandidateFilter;
  switching_tolerance: SwitchingTolerance;
  evaluation_schedule: EvaluationSchedulePolicy;
  last_evaluation_details: Record<string, unknown>;
  best_observed_path: ProxyPathSummary | null;
  best_observed_valid: boolean;
  best_observed_relative_improvement: number | null;
  best_observed_absolute_improvement_ms: number | null;
  no_switch_reason: string;
  candidate_stats: { total: number; usable: number; unknown_egress_country: number; front_candidates: number; exit_nodes: number; path_combinations: number; };
  proxy_credentials: ProxyCredentialSummary[];
  latest_switch_reason: string;
  latest_switch_at: UnixMillis | null;
  latest_switch_trigger: 'tolerance' | 'failure_recovery' | 'admin_manual' | null;
  recent_events: MaintenanceRunSummary[];
  created_at: UnixMillis;
  updated_at: UnixMillis;
};

export type AccessProfileWriteRequest = {
  name: string;
  profile_identifier: string;
  type: AccessProfileType;
  fixed_node_id?: string | null;
  exit_node_ids?: string[];
  chain_evaluation_mode?: ChainEvaluationMode | null;
  test_url?: string;
  candidate_filter?: CandidateFilter;
  switching_tolerance?: SwitchingTolerance;
  evaluation_schedule?: EvaluationSchedulePolicy;
  node_sticky_enabled?: boolean;
};

export type ProxyCredentialSummary = {
  id: string;
  access_profile_id: string;
  remark: string;
  password: string;
  http_proxy_url: string;
  https_proxy_url: string;
  socks5_proxy_url: string;
  enabled: boolean;
  created_at: UnixMillis;
  last_used_at: UnixMillis | null;
};

export type ProxyCredentialCreateRequest = { remark: string; password: string; };
export type ProxyCredentialPatchRequest = { remark?: string; enabled?: boolean; };

export type NodeState = 'enabled' | 'disabled' | 'usable' | 'unusable' | 'pending_observation';

export type NodeSummary = {
  id: string;
  name: string;
  type: string;
  protocol: string;
  server: string;
  server_port: number;
  username: string;
  password: string;
  enabled: boolean;
  state: NodeState;
  sources: Array<{ source_id: string; source_name: string; source_type: 'subscription' | 'manual'; display_name?: string }>;
  egress_ip: string | null;
  egress_country: EgressCountryDisplay;
  observation_latency_ms: number | null;
  last_observed_at: UnixMillis | null;
  last_error: string;
};

export type NodeDetail = NodeSummary & {
  raw_json?: string;
  normalized_outbound_json?: Record<string, unknown>;
  raw_source?: Record<string, unknown> | string | null;
  related_access_profiles?: AccessProfileSummary[];
};

export type NodeCreateRequest = {
  name: string;
  type: 'http' | 'socks5';
  server: string;
  server_port: number;
  username?: string;
  password?: string;
};

export type NodePatchRequest = {
  enabled?: boolean;
  name?: string;
  type?: 'http' | 'socks5';
  server?: string;
  server_port?: number;
  username?: string;
  password?: string;
  import_text?: string;
};

export type NodeUpdateResponse = {
  updated: boolean;
  id: string;
  split?: boolean;
};

export type NodeImportResponse = {
  id: string;
  ids?: string[];
  imported_nodes?: number;
  skipped_entries?: number;
  parse_error?: string;
};

export type RequestLogEntry = {
  id: string;
  occurred_at: UnixMillis;
  access_profile: { id: string; name: string; profile_identifier: string };
  proxy_credential: { id: string | null; remark: string };
  target_host: string;
  target_port: number;
  target: string;
  proxy_path: ProxyPathSummary | null;
  state: 'running' | 'completed';
  result: 'running' | 'success' | 'failure';
  success: boolean | null;
  failure_stage: 'authentication' | 'profile_selection' | 'path_selection' | 'dial' | 'proxy_handshake' | 'upstream' | '';
  error: string;
  duration_ms: number;
  ingress_bytes: number;
  egress_bytes: number;
  http_status: number | null;
};

export type ResourceCounts = {
  subscriptions: number; nodes: number; usable_nodes: number;
  access_profiles: number; proxy_credentials: number;
  requests_24h: number; failed_requests_24h: number;
};

export type GeoIPStatusSummary = {
  loaded: boolean; file_path: string; source: string;
  updated_at: UnixMillis | null; next_update_at: UnixMillis | null; last_error: string;
};

export type OverviewResponse = {
  profile_state_counts: Record<ProfileState, number>;
  access_profiles: AccessProfileSummary[];
  resource_counts: ResourceCounts;
  recent_failures: RequestLogEntry[];
  maintenance_runs: MaintenanceRunSummary[];
  geoip_status: GeoIPStatusSummary;
};

export type SystemSettings = {
  public_proxy_endpoint: string;
  log_retention_enabled: boolean;
  log_retention_days: number;
  maintenance_history_retention_enabled: boolean;
  maintenance_history_retention_days: number;
};

export type SubscriptionSummary = {
  id: string; name: string; source_type: 'remote' | 'local';
  state: 'active' | 'error' | 'disabled'; node_count: number; skipped_count: number;
  last_refresh_at: UnixMillis | null; last_error: string;
  refresh_policy: EvaluationSchedulePolicy;
};

export type SubscriptionDetail = SubscriptionSummary & {
  url: string | null; content?: string | null;
  skipped_entries: Array<{ reason: string; count: number; message: string }>;
  created_at: UnixMillis; updated_at: UnixMillis;
};

export type SubscriptionWriteRequest = {
  name: string; source_type: 'remote' | 'local';
  url?: string; content?: string;
  auto_refresh_enabled?: boolean;
  auto_refresh_interval_seconds?: number;
};

export type BuildInfo = {
  version: string;
  revision: string;
  source: string;
  license: string;
};

export type SetupStatus = { requires_setup: boolean; build?: BuildInfo; };
export type LoginResponse = { token: string; };

export type MaintenanceScheduleItem = {
  key: string; label: string; enabled: boolean; interval_seconds: number | null;
};

export type MaintenanceSettings = {
  subscription_refresh_seconds: number;
  node_observation_seconds: number;
  profile_evaluation_seconds: number;
  chain_evaluation_seconds: number;
  geoip_update_time: string;
  egress_ip_probe_url: string;
  subscription_concurrency: number;
  node_observation_concurrency: number;
  profile_evaluation_concurrency: number;
  geoip_concurrency: number;
};

export type EvaluationSettings = {
  global_concurrency: number;
  default_min_evaluation_interval_seconds: number;
  single_candidate_limit: number;
  chain_candidate_limit: number;
  connect_timeout_seconds: number;
  probe_timeout_seconds: number;
};

export type SystemSettingsResponse = SystemSettings & {
  maintenance: MaintenanceSettings;
  maintenance_schedules: MaintenanceScheduleItem[];
  switching_tolerance: SwitchingTolerance;
  geoip: GeoIPStatusSummary;
  evaluation: EvaluationSettings;
};
