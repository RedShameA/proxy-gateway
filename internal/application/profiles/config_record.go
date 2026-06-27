package profiles

import domainprofile "proxygateway/internal/domain/profile"

func (record *ConfigRecord) ApplyDefaults() {
	if record.EgressCountryMode == "" {
		record.EgressCountryMode = "include"
	}
}

func (record ConfigRecord) EffectiveProfileIdentifier() string {
	if record.ProfileIdentifier != "" {
		return record.ProfileIdentifier
	}
	return record.ID
}

func (record ConfigRecord) CandidateFilter() domainprofile.CandidateFilter {
	return domainprofile.CandidateFilter{
		EgressCountry:     record.EgressCountry,
		EgressCountries:   record.EgressCountries,
		EgressCountryMode: record.EgressCountryMode,
		NodeSourceMode:    record.NodeSourceMode,
		SourceIDs:         record.SourceIDs,
		Protocols:         record.Protocols,
		NameIncludeRegex:  record.NameIncludeRegex,
		NameExcludeRegex:  record.NameExcludeRegex,
		ManualOnly:        record.ManualOnly,
	}
}

func (record ConfigRecord) NodeStickyEnabledForType() bool {
	return record.NodeStickyEnabled && (record.Type == "fastest" || record.Type == "chain")
}

func (record ConfigRecord) DynamicStateAfterUpdate() string {
	if record.AutoEvaluationEnabled {
		return "running"
	}
	if record.CurrentNodeID != "" {
		return "ready"
	}
	return "pending"
}

func StateHasReusablePath(state string) bool {
	return state == "ready" || state == "degraded" || state == "running"
}

func (record *ConfigRecord) ApplyDomainSnapshot(snapshot domainprofile.ConfigSnapshot) {
	ApplyDomainSnapshot(record, snapshot)
}
