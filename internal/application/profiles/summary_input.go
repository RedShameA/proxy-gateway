package profiles

func SummaryInputFromConfig(record ConfigRecord, currentPath any, counts CredentialCounts) SummaryInput {
	return SummaryInput{
		ID:                      record.ID,
		Name:                    record.Name,
		Type:                    record.Type,
		State:                   record.State,
		ProfileIdentifier:       record.ProfileIdentifier,
		CurrentNodeID:           record.CurrentNodeID,
		CurrentExitNodeID:       record.CurrentExitNodeID,
		NodeSourceMode:          record.NodeSourceMode,
		SourceIDs:               record.SourceIDs,
		EgressCountry:           record.EgressCountry,
		EgressCountryMode:       record.EgressCountryMode,
		EgressCountries:         record.EgressCountries,
		NameIncludeRegex:        record.NameIncludeRegex,
		NameExcludeRegex:        record.NameExcludeRegex,
		CandidateLimit:          record.CandidateLimit,
		MinEvaluationInterval:   record.MinEvaluationIntervalSeconds,
		AutoEvaluationEnabled:   record.AutoEvaluationEnabled,
		AutoEvaluationInterval:  record.AutoEvaluationInterval,
		NodeStickyEnabled:       record.NodeStickyEnabled,
		ConfigVersion:           record.ConfigVersion,
		CurrentPath:             currentPath,
		ProxyCredentialsCount:   counts.Total,
		EnabledCredentialsCount: counts.Enabled,
		LastEvaluatedAt:         record.LastEvaluatedAt,
		LastError:               record.LastError,
		SwitchReason:            record.SwitchReason,
	}
}
