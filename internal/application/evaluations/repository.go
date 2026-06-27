package evaluations

import "context"

type Repository interface {
	ListTargets(ctx context.Context) ([]TargetRecord, error)
	LoadTarget(ctx context.Context, profileID string) (TargetRecord, bool, error)
	CurrentConfigVersion(ctx context.Context, profileID string) (int64, error)
	LastError(ctx context.Context, profileID string) (string, error)
	CurrentPathCounters(ctx context.Context, profileID string) (PathCounters, error)
	CurrentPathLatency(ctx context.Context, profileID string) (int64, error)
	CurrentNodeID(ctx context.Context, profileID string) (string, error)
	CurrentChainPath(ctx context.Context, profileID string) (ChainPath, error)
	ListCandidateNodeIDs(ctx context.Context) ([]string, error)
	CandidateEgressCountry(ctx context.Context, nodeID string) (string, bool, error)
	ListCandidateSourceRefs(ctx context.Context, nodeID string) ([]SourceRef, error)
	ProfileRetainsNode(ctx context.Context, profileID, nodeID string) (bool, error)
	UpdateProfileState(ctx context.Context, profileID string, configVersion int64, update StateUpdate) (bool, error)
	UpdateProfileStateAndReleaseRetained(ctx context.Context, profileID string, configVersion int64, keepNodeIDs []string, releaseRetained bool, update StateUpdate) (StateReleaseResult, error)
}

type TargetRecord struct {
	ID                           string
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
	CandidateLimit               int
	MinEvaluationIntervalSeconds int
	LastEvaluatedAt              int64
	ConfigVersion                int64
	RelativeImprovementThreshold float64
	AbsoluteImprovementMS        int
	NodeStickyEnabled            bool
}

type PathCounters struct {
	FailedEvaluations   int
	MissedSuccessCycles int
}

type ChainPath struct {
	FrontNodeID string
	ExitNodeID  string
}

type SourceRef struct {
	SourceType string
	SourceID   string
}

type StateUpdate struct {
	State                          *string
	LastError                      *string
	CurrentNodeID                  *string
	CurrentExitNodeID              *string
	CurrentPathLatencyMS           *int64
	CurrentPathFailedEvaluations   *int
	CurrentPathMissedSuccessCycles *int
	IncrementCurrentPathCounters   bool
	SwitchReason                   *string
	LastEvaluationDetailsJSON      *string
	LastEvaluatedAt                *int64
	LastEvaluationStartedAt        *int64
}

type StateReleaseResult struct {
	Updated             bool
	DeletedFingerprints []string
}
