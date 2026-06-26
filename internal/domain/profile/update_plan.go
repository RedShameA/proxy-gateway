package profile

import "slices"

const accessProfileChangeReason = "access_profile_change"

type ConfigSnapshot struct {
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
	CurrentPathLatencyMS         int64
	SwitchReason                 string
	LastEvaluationDetailsJSON    string
	AutoEvaluationEnabled        bool
	AutoEvaluationInterval       int
	NodeStickyEnabled            bool
	ConfigVersion                int64
}

type UpdatePlan struct {
	Config                           ConfigSnapshot
	EvaluationChanged                bool
	ResetCurrentPath                 bool
	ReleaseRetainedNodes             bool
	EnqueueEvaluation                bool
	EnqueueUnknownCountryObservation bool
}

func PlanConfigUpdate(original, updated ConfigSnapshot) UpdatePlan {
	evaluationChanged := evaluationConfigChanged(original, updated)
	resetCurrentPath := evaluationChanged && TypeNeedsEvaluation(updated.Type) && pathSelectionConfigChanged(original, updated)
	if evaluationChanged {
		updated.ConfigVersion++
	}
	if resetCurrentPath {
		updated.CurrentNodeID = ""
		updated.CurrentExitNodeID = ""
		updated.CurrentPathLatencyMS = 0
		updated.SwitchReason = accessProfileChangeReason
		updated.LastEvaluationDetailsJSON = "{}"
		updated.State = dynamicStateAfterUpdate(updated)
	}
	return UpdatePlan{
		Config:                           updated,
		EvaluationChanged:                evaluationChanged,
		ResetCurrentPath:                 resetCurrentPath,
		ReleaseRetainedNodes:             original.NodeStickyEnabled && (resetCurrentPath || !updated.NodeStickyEnabled),
		EnqueueEvaluation:                evaluationChanged && updated.AutoEvaluationEnabled && TypeNeedsEvaluation(updated.Type),
		EnqueueUnknownCountryObservation: evaluationChanged && len(updated.EgressCountries) > 0,
	}
}

func TypeNeedsEvaluation(profileType string) bool {
	return profileType == "fastest" || profileType == "chain"
}

func dynamicStateAfterUpdate(cfg ConfigSnapshot) string {
	if cfg.AutoEvaluationEnabled {
		return "running"
	}
	if cfg.CurrentNodeID != "" {
		return "ready"
	}
	return "pending"
}

func evaluationConfigChanged(original, updated ConfigSnapshot) bool {
	return updated.Type != original.Type ||
		updated.FixedNodeID != original.FixedNodeID ||
		updated.ChainEvaluationMode != original.ChainEvaluationMode ||
		updated.TestURL != original.TestURL ||
		updated.EgressCountryMode != original.EgressCountryMode ||
		updated.EgressCountry != original.EgressCountry ||
		!slices.Equal(updated.EgressCountries, original.EgressCountries) ||
		updated.NodeSourceMode != original.NodeSourceMode ||
		!slices.Equal(updated.SourceIDs, original.SourceIDs) ||
		!slices.Equal(updated.Protocols, original.Protocols) ||
		!slices.Equal(updated.ExitNodeIDs, original.ExitNodeIDs) ||
		updated.NameIncludeRegex != original.NameIncludeRegex ||
		updated.NameExcludeRegex != original.NameExcludeRegex ||
		updated.ManualOnly != original.ManualOnly ||
		updated.MinEvaluationIntervalSeconds != original.MinEvaluationIntervalSeconds ||
		updated.CandidateLimit != original.CandidateLimit ||
		updated.RelativeImprovementThreshold != original.RelativeImprovementThreshold ||
		updated.AbsoluteLatencyImprovementMS != original.AbsoluteLatencyImprovementMS ||
		updated.AutoEvaluationEnabled != original.AutoEvaluationEnabled ||
		updated.AutoEvaluationInterval != original.AutoEvaluationInterval ||
		updated.NodeStickyEnabled != original.NodeStickyEnabled
}

func pathSelectionConfigChanged(original, updated ConfigSnapshot) bool {
	return updated.Type != original.Type ||
		updated.FixedNodeID != original.FixedNodeID ||
		updated.ChainEvaluationMode != original.ChainEvaluationMode ||
		updated.EgressCountryMode != original.EgressCountryMode ||
		updated.EgressCountry != original.EgressCountry ||
		!slices.Equal(updated.EgressCountries, original.EgressCountries) ||
		updated.NodeSourceMode != original.NodeSourceMode ||
		!slices.Equal(updated.SourceIDs, original.SourceIDs) ||
		!slices.Equal(updated.Protocols, original.Protocols) ||
		!slices.Equal(updated.ExitNodeIDs, original.ExitNodeIDs) ||
		updated.NameIncludeRegex != original.NameIncludeRegex ||
		updated.NameExcludeRegex != original.NameExcludeRegex ||
		updated.ManualOnly != original.ManualOnly ||
		updated.CandidateLimit != original.CandidateLimit
}
