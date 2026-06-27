package profiles

import domainprofile "proxygateway/internal/domain/profile"

func (record ConfigRecord) DomainSnapshot() domainprofile.ConfigSnapshot {
	return domainprofile.ConfigSnapshot{
		Type:                         record.Type,
		FixedNodeID:                  record.FixedNodeID,
		ExitNodeIDs:                  record.ExitNodeIDs,
		ChainEvaluationMode:          record.ChainEvaluationMode,
		TestURL:                      record.TestURL,
		EgressCountry:                record.EgressCountry,
		EgressCountryMode:            record.EgressCountryMode,
		EgressCountries:              record.EgressCountries,
		NodeSourceMode:               record.NodeSourceMode,
		SourceIDs:                    record.SourceIDs,
		Protocols:                    record.Protocols,
		NameIncludeRegex:             record.NameIncludeRegex,
		NameExcludeRegex:             record.NameExcludeRegex,
		ManualOnly:                   record.ManualOnly,
		MinEvaluationIntervalSeconds: record.MinEvaluationIntervalSeconds,
		CandidateLimit:               record.CandidateLimit,
		RelativeImprovementThreshold: record.RelativeImprovementThreshold,
		AbsoluteLatencyImprovementMS: record.AbsoluteLatencyImprovementMS,
		CurrentNodeID:                record.CurrentNodeID,
		CurrentExitNodeID:            record.CurrentExitNodeID,
		State:                        record.State,
		CurrentPathLatencyMS:         record.CurrentPathLatencyMS,
		SwitchReason:                 record.SwitchReason,
		LastEvaluationDetailsJSON:    record.LastEvaluationDetailsJSON,
		AutoEvaluationEnabled:        record.AutoEvaluationEnabled,
		AutoEvaluationInterval:       record.AutoEvaluationInterval,
		NodeStickyEnabled:            record.NodeStickyEnabled,
		ConfigVersion:                record.ConfigVersion,
	}
}

func ApplyDomainSnapshot(record *ConfigRecord, snapshot domainprofile.ConfigSnapshot) {
	record.CurrentNodeID = snapshot.CurrentNodeID
	record.CurrentExitNodeID = snapshot.CurrentExitNodeID
	record.CurrentPathLatencyMS = snapshot.CurrentPathLatencyMS
	record.SwitchReason = snapshot.SwitchReason
	record.LastEvaluationDetailsJSON = snapshot.LastEvaluationDetailsJSON
	record.State = snapshot.State
	record.ConfigVersion = snapshot.ConfigVersion
}
