package profiles

import domainprofile "proxygateway/internal/domain/profile"

type DetailInput struct {
	Summary                      SummaryInput
	FixedNodeID                  string
	ExitNodeIDs                  []string
	ChainEvaluationMode          string
	TestURL                      string
	CurrentPathLatencyMS         int64
	LastEvaluationDetails        map[string]any
	ProxyCredentials             []Credential
	CandidateStats               CandidateStats
	RecentEvents                 []map[string]any
	CandidateFilterSourceMode    string
	SourceIDs                    []string
	Protocols                    []string
	EgressCountries              []string
	RelativeImprovementThreshold float64
	AbsoluteLatencyImprovementMS int
}

type Credential struct {
	ID              string `json:"id"`
	AccessProfileID string `json:"access_profile_id"`
	Remark          string `json:"remark"`
	Password        string `json:"password"`
	Enabled         bool   `json:"enabled"`
	CreatedAt       int64  `json:"created_at"`
	LastUsedAt      int64  `json:"last_used_at"`
	HTTPProxyURL    string `json:"http_proxy_url"`
	HTTPSProxyURL   string `json:"https_proxy_url"`
	SOCKS5ProxyURL  string `json:"socks5_proxy_url"`
}

type CandidateStats struct {
	Total                int `json:"total"`
	Usable               int `json:"usable"`
	UnknownEgressCountry int `json:"unknown_egress_country"`
	FrontCandidates      int `json:"front_candidates"`
	ExitNodes            int `json:"exit_nodes"`
	PathCombinations     int `json:"path_combinations"`
}

func BuildCandidateStats(profileType string, candidateNodeIDs []string, usableCount, unknownEgressCountryCount int, exitNodeIDs []string) CandidateStats {
	stats := domainprofile.BuildCandidateStats(profileType, candidateNodeIDs, usableCount, unknownEgressCountryCount, exitNodeIDs)
	return CandidateStats{
		Total:                stats.Total,
		Usable:               stats.Usable,
		UnknownEgressCountry: stats.UnknownEgressCountry,
		FrontCandidates:      stats.FrontCandidates,
		ExitNodes:            stats.ExitNodes,
		PathCombinations:     stats.PathCombinations,
	}
}

type CandidateFilter struct {
	SourceMode        string   `json:"source_mode"`
	SourceIDs         []string `json:"source_ids"`
	Protocols         []string `json:"protocols"`
	NameInclude       string   `json:"name_include"`
	NameExclude       string   `json:"name_exclude"`
	EgressCountryMode string   `json:"egress_country_mode"`
	EgressCountries   []string `json:"egress_countries"`
}

type SwitchingTolerance struct {
	RelativeImprovementThreshold float64 `json:"relative_improvement_threshold"`
	AbsoluteLatencyImprovementMS int     `json:"absolute_latency_improvement_ms"`
}

type EvaluationSchedule struct {
	Mode            string `json:"mode"`
	IntervalSeconds int    `json:"interval_seconds"`
}

type Detail struct {
	Summary
	FixedNodeID                       any                `json:"fixed_node_id"`
	ExitNodeIDs                       []string           `json:"exit_node_ids"`
	ChainEvaluationMode               any                `json:"chain_evaluation_mode"`
	TestURL                           string             `json:"test_url"`
	CurrentPathLatencyMS              any                `json:"current_path_latency_ms"`
	LastEvaluationDetails             map[string]any     `json:"last_evaluation_details"`
	ProxyCredentials                  []Credential       `json:"proxy_credentials"`
	BestObservedPath                  any                `json:"best_observed_path"`
	BestObservedValid                 bool               `json:"best_observed_valid"`
	BestObservedRelativeImprovement   any                `json:"best_observed_relative_improvement"`
	BestObservedAbsoluteImprovementMS any                `json:"best_observed_absolute_improvement_ms"`
	NoSwitchReason                    string             `json:"no_switch_reason"`
	CandidateStats                    CandidateStats     `json:"candidate_stats"`
	LatestSwitchReason                string             `json:"latest_switch_reason"`
	LatestSwitchAt                    any                `json:"latest_switch_at"`
	LatestSwitchTrigger               any                `json:"latest_switch_trigger"`
	RecentEvents                      []map[string]any   `json:"recent_events"`
	CandidateFilter                   CandidateFilter    `json:"candidate_filter"`
	SwitchingTolerance                SwitchingTolerance `json:"switching_tolerance"`
	EvaluationSchedule                EvaluationSchedule `json:"evaluation_schedule"`
	CreatedAt                         int64              `json:"created_at"`
	UpdatedAt                         int64              `json:"updated_at"`
}

func BuildDetail(input DetailInput) Detail {
	summary := BuildSummary(input.Summary)
	fixedNodeID := any(nil)
	if input.FixedNodeID != "" {
		fixedNodeID = input.FixedNodeID
	}
	chainEvaluationMode := any(nil)
	if input.Summary.Type == domainprofile.TypeChain {
		chainEvaluationMode = domainprofile.NormalizeChainEvaluationMode(input.ChainEvaluationMode)
	}
	lastEvaluationDetails := input.LastEvaluationDetails
	if lastEvaluationDetails == nil {
		lastEvaluationDetails = map[string]any{}
	}
	proxyCredentials := input.ProxyCredentials
	if proxyCredentials == nil {
		proxyCredentials = []Credential{}
	}
	recentEvents := input.RecentEvents
	if recentEvents == nil {
		recentEvents = []map[string]any{}
	}
	candidateFilterSourceMode := input.CandidateFilterSourceMode
	if candidateFilterSourceMode == "" {
		candidateFilterSourceMode = input.Summary.NodeSourceMode
	}
	return Detail{
		Summary:                           summary,
		FixedNodeID:                       fixedNodeID,
		ExitNodeIDs:                       nonNilStrings(input.ExitNodeIDs),
		ChainEvaluationMode:               chainEvaluationMode,
		TestURL:                           input.TestURL,
		CurrentPathLatencyMS:              nullableUnixMillis(input.CurrentPathLatencyMS),
		LastEvaluationDetails:             lastEvaluationDetails,
		ProxyCredentials:                  proxyCredentials,
		BestObservedPath:                  nil,
		BestObservedValid:                 false,
		BestObservedRelativeImprovement:   nil,
		BestObservedAbsoluteImprovementMS: nil,
		NoSwitchReason:                    "",
		CandidateStats:                    input.CandidateStats,
		LatestSwitchReason:                "",
		LatestSwitchAt:                    nil,
		LatestSwitchTrigger:               nil,
		RecentEvents:                      recentEvents,
		CandidateFilter: CandidateFilter{
			SourceMode:        candidateFilterSourceMode,
			SourceIDs:         nonNilStrings(input.SourceIDs),
			Protocols:         nonNilStrings(input.Protocols),
			NameInclude:       input.Summary.NameIncludeRegex,
			NameExclude:       input.Summary.NameExcludeRegex,
			EgressCountryMode: input.Summary.EgressCountryMode,
			EgressCountries:   nonNilStrings(input.EgressCountries),
		},
		SwitchingTolerance: SwitchingTolerance{
			RelativeImprovementThreshold: input.RelativeImprovementThreshold,
			AbsoluteLatencyImprovementMS: input.AbsoluteLatencyImprovementMS,
		},
		EvaluationSchedule: EvaluationSchedule{
			Mode:            evaluationScheduleMode(input.Summary.AutoEvaluationEnabled),
			IntervalSeconds: input.Summary.AutoEvaluationInterval,
		},
		CreatedAt: 0,
		UpdatedAt: 0,
	}
}

func evaluationScheduleMode(enabled bool) string {
	if enabled {
		return ScheduleModeCustom
	}
	return ScheduleModeDisabled
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
