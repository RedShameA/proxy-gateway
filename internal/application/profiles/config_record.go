package profiles

import domainprofile "proxygateway/internal/domain/profile"

func (record *ConfigRecord) ApplyDefaults() {
	if record.EgressCountryMode == "" {
		record.EgressCountryMode = domainprofile.EgressCountryModeInclude
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
	return record.NodeStickyEnabled && (record.Type == domainprofile.TypeFastest || record.Type == domainprofile.TypeChain)
}

func (record ConfigRecord) DynamicStateAfterUpdate() string {
	if record.AutoEvaluationEnabled {
		return domainprofile.StateRunning
	}
	if record.CurrentNodeID != "" {
		return domainprofile.StateReady
	}
	return domainprofile.StatePending
}

func StateHasReusablePath(state string) bool {
	return state == domainprofile.StateReady || state == domainprofile.StateDegraded || state == domainprofile.StateRunning
}

func (record *ConfigRecord) ApplyDomainSnapshot(snapshot domainprofile.ConfigSnapshot) {
	ApplyDomainSnapshot(record, snapshot)
}
