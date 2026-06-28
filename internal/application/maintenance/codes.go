package maintenance

const (
	RunTypeSubscriptionRefresh = "subscription_refresh"
	RunTypeNodeObservation     = "node_observation"
	RunTypeProfileEvaluation   = "profile_evaluation"
	RunTypeProfileSwitch       = "profile_switch"
	RunTypeGeoIPUpdate         = "geoip_update"
	RunTypeLogCleanup          = "log_cleanup"
	RunTypeStartupCleanup      = "startup_cleanup"
)

const (
	StateQueued   = "queued"
	StateRunning  = "running"
	StateFinished = "finished"
)

const (
	ResultSuccess   = "success"
	ResultWarning   = "warning"
	ResultFailure   = "failure"
	ResultSkipped   = "skipped"
	ResultCancelled = "cancelled"
)

const (
	TriggerScheduled                    = "scheduled"
	TriggerManual                       = "manual"
	TriggerStartup                      = "startup"
	TriggerAccessProfileChange          = "access_profile_change"
	TriggerNodeObservation              = "node_observation"
	TriggerSubscriptionRefresh          = "subscription_refresh"
	TriggerSubscriptionImport           = "subscription_import"
	TriggerCurrentNodeRemoved           = "current_node_removed"
	TriggerCurrentNodeObserved          = "current_node_observed"
	TriggerCountryProfileUnknownCountry = "country_profile_unknown_country"
	TriggerManualNodeImport             = "manual_node_import"
	TriggerPendingRerun                 = "pending_rerun"
	TriggerConfigChange                 = "config_change"
)

const (
	ReasonCompleted                       = "completed"
	ReasonPartialFailure                  = "partial_failure"
	ReasonAllFailed                       = "all_failed"
	ReasonNoTargets                       = "no_targets"
	ReasonPreviousRunStillRunning         = "previous_run_still_running"
	ReasonWaitingForObservation           = "waiting_for_observation"
	ReasonReplacedByManualRun             = "replaced_by_manual_run"
	ReasonExpiredAfterRestart             = "expired_after_restart"
	ReasonMinIntervalNotReached           = "min_interval_not_reached"
	ReasonSupersededByConfigVersion       = "superseded_by_config_version"
	ReasonProfileLoadFailed               = "profile_load_failed"
	ReasonProfileTypeNotEvaluable         = "profile_type_not_evaluable"
	ReasonCurrentPathDegraded             = "current_path_degraded"
	ReasonEvaluationFailed                = "evaluation_failed"
	ReasonUnknownRunType                  = "unknown_run_type"
	ReasonSubscriptionNotFound            = "subscription_not_found"
	ReasonFetchFailed                     = "fetch_failed"
	ReasonParseFailed                     = "parse_failed"
	ReasonImportFailed                    = "import_failed"
	ReasonNoImportableNodes               = "no_importable_nodes"
	ReasonGeoIPServiceUnavailable         = "geoip_service_unavailable"
	ReasonGeoIPUpdateFailed               = "geoip_update_failed"
	ReasonRequestLogCleanupFailed         = "request_log_cleanup_failed"
	ReasonMaintenanceHistoryCleanupFailed = "maintenance_history_cleanup_failed"
)

const (
	NodeObservationScopeAllNodes   = "all_nodes"
	NodeObservationScopeDueNodes   = "due_nodes"
	NodeObservationScopeSingleNode = "single_node"
)
