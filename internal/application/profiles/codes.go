package profiles

import domainprofile "proxygateway/internal/domain/profile"

const (
	ActionEvaluate             = "evaluate"
	ActionSwitchToBestObserved = "switch-to-best-observed"
	ManualSwitchReason         = domainprofile.SwitchReasonManualSwitchRequested
)

const (
	APINodeSourceModeAll             = "all"
	APINodeSourceModeManual          = "manual"
	APINodeSourceModeSubscription    = "subscription"
	APINodeSourceModeSelectedSources = "selected_sources"
)

const (
	ScheduleModeInherit  = "inherit"
	ScheduleModeCustom   = "custom"
	ScheduleModeDisabled = "disabled"
)
