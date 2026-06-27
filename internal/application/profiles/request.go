package profiles

type PatchRequest struct {
	Name                *string                  `json:"name"`
	ProfileIdentifier   *string                  `json:"profile_identifier"`
	Type                *string                  `json:"type"`
	FixedNodeID         *string                  `json:"fixed_node_id"`
	ExitNodeIDs         *[]string                `json:"exit_node_ids"`
	ChainEvaluationMode *string                  `json:"chain_evaluation_mode"`
	TestURL             *string                  `json:"test_url"`
	CandidateFilter     *PatchCandidateFilter    `json:"candidate_filter"`
	SwitchingTolerance  *PatchSwitchingTolerance `json:"switching_tolerance"`
	EvaluationSchedule  *PatchEvaluationSchedule `json:"evaluation_schedule"`
	EgressCountry       *string                  `json:"egress_country"`
	EgressCountryMode   *string                  `json:"egress_country_mode"`
	EgressCountries     *[]string                `json:"egress_countries"`
	NodeSourceMode      *string                  `json:"node_source_mode"`
	SourceIDs           *[]string                `json:"source_ids"`
	Protocols           *[]string                `json:"protocols"`
	NameIncludeRegex    *string                  `json:"name_include_regex"`
	NameExcludeRegex    *string                  `json:"name_exclude_regex"`
	ManualOnly          *bool                    `json:"manual_only"`
	CandidateLimit      *int                     `json:"candidate_limit"`
	MinEvalInterval     *int                     `json:"min_evaluation_interval_seconds"`
	AutoEvalEnabled     *bool                    `json:"auto_evaluation_enabled"`
	AutoEvalInterval    *int                     `json:"auto_evaluation_interval_seconds"`
	NodeStickyEnabled   *bool                    `json:"node_sticky_enabled"`
}

type PatchCandidateFilter struct {
	SourceMode        string   `json:"source_mode"`
	SourceIDs         []string `json:"source_ids"`
	Protocols         []string `json:"protocols"`
	NameInclude       string   `json:"name_include"`
	NameExclude       string   `json:"name_exclude"`
	EgressCountryMode string   `json:"egress_country_mode"`
	EgressCountries   []string `json:"egress_countries"`
}

type PatchSwitchingTolerance struct {
	RelativeImprovementThreshold *float64 `json:"relative_improvement_threshold"`
	AbsoluteLatencyImprovementMS *int     `json:"absolute_latency_improvement_ms"`
}

type PatchEvaluationSchedule struct {
	Mode            string `json:"mode"`
	IntervalSeconds *int   `json:"interval_seconds"`
}

func (req PatchRequest) IsEmpty() bool {
	return req.Name == nil &&
		req.ProfileIdentifier == nil &&
		req.Type == nil &&
		req.FixedNodeID == nil &&
		req.ExitNodeIDs == nil &&
		req.ChainEvaluationMode == nil &&
		req.TestURL == nil &&
		req.CandidateFilter == nil &&
		req.SwitchingTolerance == nil &&
		req.EvaluationSchedule == nil &&
		req.EgressCountry == nil &&
		req.EgressCountryMode == nil &&
		req.EgressCountries == nil &&
		req.NodeSourceMode == nil &&
		req.SourceIDs == nil &&
		req.Protocols == nil &&
		req.NameIncludeRegex == nil &&
		req.NameExcludeRegex == nil &&
		req.ManualOnly == nil &&
		req.CandidateLimit == nil &&
		req.MinEvalInterval == nil &&
		req.AutoEvalEnabled == nil &&
		req.AutoEvalInterval == nil &&
		req.NodeStickyEnabled == nil
}
