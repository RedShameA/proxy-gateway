package evaluations

import domainprofile "proxygateway/internal/domain/profile"

const (
	ProfileStatePending            = domainprofile.StatePending
	ProfileStateRunning            = domainprofile.StateRunning
	ProfileStateWaitingObservation = domainprofile.StateWaitingObservation
	ProfileStateReady              = domainprofile.StateReady
	ProfileStateDegraded           = domainprofile.StateDegraded
	ProfileStateNoCandidate        = domainprofile.StateNoCandidate
	ProfileStateFailed             = domainprofile.StateFailed
	ProfileStateInvalidConfig      = domainprofile.StateInvalidConfig
)

const (
	SwitchReasonInitialSelection              = domainprofile.SwitchReasonInitialSelection
	SwitchReasonForceSwitch                   = domainprofile.SwitchReasonForceSwitch
	SwitchReasonCurrentPathStillBest          = domainprofile.SwitchReasonCurrentPathStillBest
	SwitchReasonCandidateNotClearlyBetter     = domainprofile.SwitchReasonCandidateNotClearlyBetter
	SwitchReasonCandidateClearlyBetter        = domainprofile.SwitchReasonCandidateClearlyBetter
	SwitchReasonCurrentPathFailedSwitch       = domainprofile.SwitchReasonCurrentPathFailedSwitch
	SwitchReasonCurrentPathReusedAfterFailure = domainprofile.SwitchReasonCurrentPathReusedAfterFailure
	SwitchReasonAllCandidatesFailed           = domainprofile.SwitchReasonAllCandidatesFailed
	SwitchReasonNoCandidate                   = domainprofile.SwitchReasonNoCandidate
	SwitchReasonCandidateFilterError          = domainprofile.SwitchReasonCandidateFilterError
	SwitchReasonInvalidChainConfig            = domainprofile.SwitchReasonInvalidChainConfig
	SwitchReasonMissingExitNode               = domainprofile.SwitchReasonMissingExitNode
	SwitchReasonSelectedNodeRemoved           = domainprofile.SwitchReasonSelectedNodeRemoved
	SwitchReasonCurrentNodeRemoved            = domainprofile.SwitchReasonCurrentNodeRemoved
	SwitchReasonMissingFixedNode              = domainprofile.SwitchReasonMissingFixedNode
	SwitchReasonAccessProfileChange           = domainprofile.SwitchReasonAccessProfileChange
	SwitchReasonManualSwitchRequested         = domainprofile.SwitchReasonManualSwitchRequested
)
