package observations

type CountryLookup interface {
	LookupCountry(ip string) string
}

type PersistenceRepository interface {
	SaveSuccess(nodeID string, record SuccessRecord, observedAt int64) error
	SaveFailure(nodeID, errorText string, observedAt int64) error
}

func PersistSuccess(repo PersistenceRepository, lookup CountryLookup, nodeID string, raw []byte, latencyMS, observedAt int64) error {
	response := ParseProbeResponse(raw)
	country := ""
	if lookup != nil {
		country = lookup.LookupCountry(response.EgressIP)
	}
	return repo.SaveSuccess(nodeID, BuildSuccessRecord(response, country, latencyMS), observedAt)
}

func PersistFailure(repo PersistenceRepository, nodeID, errorText string, observedAt int64) error {
	return repo.SaveFailure(nodeID, errorText, observedAt)
}
