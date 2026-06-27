package profiles

import (
	"strings"

	domainprofile "proxygateway/internal/domain/profile"
)

func DefaultConfig(id string) ConfigRecord {
	return ConfigRecord{
		ID:                           id,
		Type:                         "fastest",
		EgressCountryMode:            "include",
		NodeSourceMode:               "all",
		RelativeImprovementThreshold: 0.2,
		AbsoluteLatencyImprovementMS: 100,
		State:                        "pending",
		AutoEvaluationEnabled:        true,
		ConfigVersion:                1,
	}
}

func ApplyConfigPatch(cfg *ConfigRecord, req PatchRequest) {
	if req.Name != nil {
		cfg.Name = *req.Name
	}
	if req.ProfileIdentifier != nil {
		cfg.ProfileIdentifier = *req.ProfileIdentifier
	}
	if req.Type != nil {
		cfg.Type = *req.Type
	}
	if req.FixedNodeID != nil {
		cfg.FixedNodeID = *req.FixedNodeID
	}
	if req.ExitNodeIDs != nil {
		cfg.ExitNodeIDs = *req.ExitNodeIDs
	}
	if req.ChainEvaluationMode != nil {
		cfg.ChainEvaluationMode = *req.ChainEvaluationMode
	}
	if req.TestURL != nil {
		cfg.TestURL = *req.TestURL
	}
	if req.CandidateFilter != nil {
		filter := req.CandidateFilter
		cfg.NodeSourceMode = domainprofile.NormalizeNodeSourceMode(filter.SourceMode, nil, false)
		cfg.SourceIDs = filter.SourceIDs
		cfg.Protocols = filter.Protocols
		cfg.NameIncludeRegex = filter.NameInclude
		cfg.NameExcludeRegex = filter.NameExclude
		cfg.EgressCountryMode = filter.EgressCountryMode
		cfg.EgressCountries = filter.EgressCountries
	}
	if req.SwitchingTolerance != nil {
		if req.SwitchingTolerance.RelativeImprovementThreshold != nil {
			cfg.RelativeImprovementThreshold = *req.SwitchingTolerance.RelativeImprovementThreshold
		}
		if req.SwitchingTolerance.AbsoluteLatencyImprovementMS != nil {
			cfg.AbsoluteLatencyImprovementMS = *req.SwitchingTolerance.AbsoluteLatencyImprovementMS
		}
	}
	if req.EvaluationSchedule != nil {
		applyEvaluationSchedulePatch(cfg, *req.EvaluationSchedule)
	}
	if req.EgressCountry != nil {
		cfg.EgressCountry = *req.EgressCountry
	}
	if req.EgressCountryMode != nil {
		cfg.EgressCountryMode = *req.EgressCountryMode
	}
	if req.EgressCountries != nil {
		cfg.EgressCountries = *req.EgressCountries
	}
	if req.NodeSourceMode != nil {
		cfg.NodeSourceMode = *req.NodeSourceMode
	}
	if req.SourceIDs != nil {
		cfg.SourceIDs = *req.SourceIDs
	}
	if req.Protocols != nil {
		cfg.Protocols = *req.Protocols
	}
	if req.NameIncludeRegex != nil {
		cfg.NameIncludeRegex = *req.NameIncludeRegex
	}
	if req.NameExcludeRegex != nil {
		cfg.NameExcludeRegex = *req.NameExcludeRegex
	}
	if req.ManualOnly != nil {
		cfg.ManualOnly = *req.ManualOnly
	}
	if req.CandidateLimit != nil {
		cfg.CandidateLimit = *req.CandidateLimit
	}
	if req.MinEvalInterval != nil {
		cfg.MinEvaluationIntervalSeconds = *req.MinEvalInterval
	}
	if req.AutoEvalEnabled != nil {
		cfg.AutoEvaluationEnabled = *req.AutoEvalEnabled
	}
	if req.AutoEvalInterval != nil {
		cfg.AutoEvaluationInterval = *req.AutoEvalInterval
	}
	if req.NodeStickyEnabled != nil {
		cfg.NodeStickyEnabled = *req.NodeStickyEnabled
	}
}

func applyEvaluationSchedulePatch(cfg *ConfigRecord, schedule PatchEvaluationSchedule) {
	switch strings.ToLower(strings.TrimSpace(schedule.Mode)) {
	case "disabled":
		cfg.AutoEvaluationEnabled = false
	case "custom":
		cfg.AutoEvaluationEnabled = true
		if schedule.IntervalSeconds != nil {
			cfg.AutoEvaluationInterval = *schedule.IntervalSeconds
		}
	case "inherit", "":
		cfg.AutoEvaluationEnabled = true
		if schedule.Mode == "inherit" {
			cfg.AutoEvaluationInterval = 0
		} else if schedule.IntervalSeconds != nil {
			cfg.AutoEvaluationInterval = *schedule.IntervalSeconds
		}
	}
}
