package subscriptions

const (
	SourceTypeRemote = "remote"
	SourceTypeLocal  = "local"
)

const (
	StateActive   = "active"
	StateError    = "error"
	StateDisabled = "disabled"
)

const (
	RefreshPolicyModeInherit  = "inherit"
	RefreshPolicyModeCustom   = "custom"
	RefreshPolicyModeDisabled = "disabled"
)

const (
	SkippedReasonUnsupportedFunctionalOutbound = "unsupported_functional_outbound"
	SkippedReasonMissingRequiredField          = "missing_required_field"
	SkippedReasonMalformedEntry                = "malformed_entry"
	SkippedReasonUnsupportedNodeType           = "unsupported_node_type"
	SkippedReasonUnsupportedOption             = "unsupported_option"
	SkippedReasonClashProxyGroupIgnored        = "clash_proxy_group_ignored"
	SkippedReasonDuplicateNode                 = "duplicate_node"
)

const (
	SummaryTypeIgnored = "ignored"
	SummaryTypeSkipped = "skipped"
)
