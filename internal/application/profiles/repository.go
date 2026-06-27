package profiles

import "context"

type ConfigRecord struct {
	ID                           string
	Name                         string
	ProfileIdentifier            string
	Type                         string
	FixedNodeID                  string
	ExitNodeIDs                  []string
	ChainEvaluationMode          string
	TestURL                      string
	EgressCountry                string
	EgressCountryMode            string
	EgressCountries              []string
	NodeSourceMode               string
	SourceIDs                    []string
	Protocols                    []string
	NameIncludeRegex             string
	NameExcludeRegex             string
	ManualOnly                   bool
	MinEvaluationIntervalSeconds int
	CandidateLimit               int
	RelativeImprovementThreshold float64
	AbsoluteLatencyImprovementMS int
	CurrentNodeID                string
	CurrentExitNodeID            string
	State                        string
	LastEvaluatedAt              int64
	LastError                    string
	CurrentPathLatencyMS         int64
	SwitchReason                 string
	LastEvaluationDetailsJSON    string
	AutoEvaluationEnabled        bool
	AutoEvaluationInterval       int
	NodeStickyEnabled            bool
	ConfigVersion                int64
}

type ListConfigFilter struct {
	Limit  int
	Offset int
}

type ListConfigResult struct {
	IDs   []string
	Total int
}

type ConfigUpdateOptions struct {
	EvaluationChanged bool
	ResetCurrentPath  bool
}

type ConfigRepository interface {
	ListConfigIDs(ctx context.Context, filter ListConfigFilter) (ListConfigResult, error)
	LoadConfig(ctx context.Context, profileID string) (ConfigRecord, bool, error)
	CreateConfig(ctx context.Context, record ConfigRecord, createdAt int64) error
	UpdateConfig(ctx context.Context, record ConfigRecord, options ConfigUpdateOptions) error
	ProfileIdentifierExists(ctx context.Context, identifier, excludeProfileID string) (bool, error)
	Exists(ctx context.Context, profileID string) (bool, error)
}

type ConfigUpdater interface {
	UpdateConfig(ctx context.Context, record ConfigRecord, options ConfigUpdateOptions) error
}
