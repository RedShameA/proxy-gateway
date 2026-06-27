package profiles

type SummaryInput struct {
	ID                      string
	Name                    string
	Type                    string
	State                   string
	ProfileIdentifier       string
	CurrentNodeID           string
	CurrentExitNodeID       string
	NodeSourceMode          string
	SourceIDs               []string
	EgressCountry           string
	EgressCountryMode       string
	EgressCountries         []string
	NameIncludeRegex        string
	NameExcludeRegex        string
	CandidateLimit          int
	MinEvaluationInterval   int
	AutoEvaluationEnabled   bool
	AutoEvaluationInterval  int
	NodeStickyEnabled       bool
	ConfigVersion           int64
	CurrentPath             any
	ProxyCredentialsCount   int
	EnabledCredentialsCount int
	LastEvaluatedAt         int64
	LastError               string
	SwitchReason            string
}

type Summary struct {
	ID                           string   `json:"id"`
	Name                         string   `json:"name"`
	Type                         string   `json:"type"`
	State                        string   `json:"state"`
	ProfileIdentifier            string   `json:"profile_identifier"`
	CurrentNodeID                string   `json:"current_node_id"`
	CurrentExitNodeID            string   `json:"current_exit_node_id"`
	NodeSourceMode               string   `json:"node_source_mode"`
	SourceIDs                    []string `json:"source_ids"`
	EgressCountry                string   `json:"egress_country"`
	EgressCountryMode            string   `json:"egress_country_mode"`
	EgressCountries              []string `json:"egress_countries"`
	NameIncludeRegex             string   `json:"name_include_regex"`
	NameExcludeRegex             string   `json:"name_exclude_regex"`
	CandidateLimit               int      `json:"candidate_limit"`
	MinEvaluationInterval        int      `json:"min_evaluation_interval_seconds"`
	AutoEvaluationEnabled        bool     `json:"auto_evaluation_enabled"`
	AutoEvaluationInterval       int      `json:"auto_evaluation_interval_seconds"`
	NodeStickyEnabled            bool     `json:"node_sticky_enabled"`
	ConfigVersion                int64    `json:"config_version"`
	CurrentPath                  any      `json:"current_path"`
	ProxyCredentialsCount        int      `json:"proxy_credentials_count"`
	EnabledProxyCredentialsCount int      `json:"enabled_proxy_credentials_count"`
	LastEvaluatedAt              any      `json:"last_evaluated_at"`
	LastError                    string   `json:"last_error"`
	SwitchReason                 string   `json:"switch_reason"`
}

type SummaryList struct {
	Items          []Summary `json:"items"`
	AccessProfiles []Summary `json:"access_profiles"`
	Total          int       `json:"total"`
}

func BuildSummary(input SummaryInput) Summary {
	profileIdentifier := input.ProfileIdentifier
	if profileIdentifier == "" {
		profileIdentifier = input.ID
	}
	return Summary{
		ID:                           input.ID,
		Name:                         input.Name,
		Type:                         input.Type,
		State:                        input.State,
		ProfileIdentifier:            profileIdentifier,
		CurrentNodeID:                input.CurrentNodeID,
		CurrentExitNodeID:            input.CurrentExitNodeID,
		NodeSourceMode:               input.NodeSourceMode,
		SourceIDs:                    input.SourceIDs,
		EgressCountry:                input.EgressCountry,
		EgressCountryMode:            input.EgressCountryMode,
		EgressCountries:              input.EgressCountries,
		NameIncludeRegex:             input.NameIncludeRegex,
		NameExcludeRegex:             input.NameExcludeRegex,
		CandidateLimit:               input.CandidateLimit,
		MinEvaluationInterval:        input.MinEvaluationInterval,
		AutoEvaluationEnabled:        input.AutoEvaluationEnabled,
		AutoEvaluationInterval:       input.AutoEvaluationInterval,
		NodeStickyEnabled:            input.NodeStickyEnabled && (input.Type == "fastest" || input.Type == "chain"),
		ConfigVersion:                input.ConfigVersion,
		CurrentPath:                  input.CurrentPath,
		ProxyCredentialsCount:        input.ProxyCredentialsCount,
		EnabledProxyCredentialsCount: input.EnabledCredentialsCount,
		LastEvaluatedAt:              nullableUnixMillis(input.LastEvaluatedAt),
		LastError:                    input.LastError,
		SwitchReason:                 input.SwitchReason,
	}
}

func BuildSummaryList(items []Summary, total int) SummaryList {
	if items == nil {
		items = []Summary{}
	}
	return SummaryList{
		Items:          items,
		AccessProfiles: items,
		Total:          total,
	}
}

func nullableUnixMillis(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
