package evaluations

import domainprofile "proxygateway/internal/domain/profile"

type TargetBuildInput struct {
	Record                              TargetRecord
	DefaultTestURL                      string
	DefaultMinEvaluationIntervalSeconds int
	NowMS                               int64
	ForceSwitch                         bool
}

type Target struct {
	ID                           string
	Type                         string
	FixedNodeID                  string
	ExitNodeIDs                  []string
	ChainEvaluationMode          string
	TestURL                      string
	Filter                       domainprofile.CandidateFilter
	CandidateLimit               int
	MinEvaluationIntervalSeconds int
	RelativeImprovementThreshold float64
	AbsoluteImprovementMS        int
	LastEvaluatedAt              int64
	ConfigVersion                int64
	ForceSwitch                  bool
	AutoEvaluationEnabled        bool
	NodeStickyEnabled            bool
}

func BuildTarget(input TargetBuildInput) (Target, bool) {
	record := input.Record
	snapshot, shouldSkip := BuildTargetSnapshot(TargetSnapshotInput{
		FixedNodeID:                         record.FixedNodeID,
		ExitNodeIDs:                         record.ExitNodeIDs,
		TestURL:                             record.TestURL,
		DefaultTestURL:                      input.DefaultTestURL,
		LastEvaluatedAt:                     record.LastEvaluatedAt,
		MinEvaluationIntervalSeconds:        record.MinEvaluationIntervalSeconds,
		DefaultMinEvaluationIntervalSeconds: input.DefaultMinEvaluationIntervalSeconds,
		NowMS:                               input.NowMS,
		ForceSwitch:                         input.ForceSwitch,
	})
	if shouldSkip {
		return Target{}, true
	}
	return Target{
		ID:                           record.ID,
		Type:                         record.Type,
		FixedNodeID:                  record.FixedNodeID,
		ExitNodeIDs:                  snapshot.ExitNodeIDs,
		ChainEvaluationMode:          domainprofile.NormalizeChainEvaluationMode(record.ChainEvaluationMode),
		TestURL:                      snapshot.TestURL,
		Filter:                       targetCandidateFilter(record),
		CandidateLimit:               record.CandidateLimit,
		MinEvaluationIntervalSeconds: record.MinEvaluationIntervalSeconds,
		RelativeImprovementThreshold: record.RelativeImprovementThreshold,
		AbsoluteImprovementMS:        record.AbsoluteImprovementMS,
		LastEvaluatedAt:              record.LastEvaluatedAt,
		ConfigVersion:                record.ConfigVersion,
		ForceSwitch:                  input.ForceSwitch,
		NodeStickyEnabled:            record.NodeStickyEnabled,
	}, false
}

func TypeNeedsEvaluation(profileType string) bool {
	return domainprofile.TypeNeedsEvaluation(profileType)
}

func targetCandidateFilter(record TargetRecord) domainprofile.CandidateFilter {
	return domainprofile.NormalizeCandidateFilter(domainprofile.CandidateFilter{
		EgressCountry:     record.EgressCountry,
		EgressCountries:   record.EgressCountries,
		EgressCountryMode: record.EgressCountryMode,
		NodeSourceMode:    record.NodeSourceMode,
		SourceIDs:         record.SourceIDs,
		Protocols:         record.Protocols,
		NameIncludeRegex:  record.NameIncludeRegex,
		NameExcludeRegex:  record.NameExcludeRegex,
		ManualOnly:        record.ManualOnly,
	})
}
