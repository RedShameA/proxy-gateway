package profile

const (
	TypeFixedNode = "fixed_node"
	TypeFastest   = "fastest"
	TypeRandom    = "random"
	TypeChain     = "chain"
)

const (
	ChainEvaluationModeChainLink = "chain_link"
	ChainEvaluationModeEndToEnd  = "end_to_end"
)

const (
	NodeSourceModeAll                   = "all"
	NodeSourceModeManual                = "manual"
	NodeSourceModeSubscriptions         = "subscriptions"
	NodeSourceModeSpecificSubscriptions = "specific_subscriptions"
)

const (
	NodeSourceModeAliasSubscription    = "subscription"
	NodeSourceModeAliasSelectedSources = "selected_sources"
)

const (
	EgressCountryModeInclude = "include"
	EgressCountryModeExclude = "exclude"
)

const (
	StatePending            = "pending"
	StateRunning            = "running"
	StateWaitingObservation = "waiting_observation"
	StateReady              = "ready"
	StateDegraded           = "degraded"
	StateNoCandidate        = "no_candidate"
	StateFailed             = "failed"
	StateInvalidConfig      = "invalid_config"
)

const (
	SwitchReasonInitialSelection              = "initial_selection"
	SwitchReasonForceSwitch                   = "force_switch"
	SwitchReasonCurrentPathStillBest          = "current_path_still_best"
	SwitchReasonCandidateNotClearlyBetter     = "candidate_not_clearly_better"
	SwitchReasonCandidateClearlyBetter        = "candidate_clearly_better"
	SwitchReasonCurrentPathFailedSwitch       = "current_path_failed_switch"
	SwitchReasonCurrentPathReusedAfterFailure = "current_path_reused_after_failure"
	SwitchReasonAllCandidatesFailed           = "all_candidates_failed"
	SwitchReasonNoCandidate                   = "no_candidate"
	SwitchReasonCandidateFilterError          = "candidate_filter_error"
	SwitchReasonInvalidChainConfig            = "invalid_chain_config"
	SwitchReasonMissingExitNode               = "missing_exit_node"
	SwitchReasonSelectedNodeRemoved           = "selected_node_removed"
	SwitchReasonCurrentNodeRemoved            = "current_node_removed"
	SwitchReasonMissingFixedNode              = "missing_fixed_node"
	SwitchReasonAccessProfileChange           = "access_profile_change"
	SwitchReasonManualSwitchRequested         = "manual_switch_requested"
)
