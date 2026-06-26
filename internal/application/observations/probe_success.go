package observations

import (
	"encoding/json"
	"strings"
)

type ProbeResponse struct {
	EgressIP     string
	ProbeCountry string
}

type SuccessRecord struct {
	EgressIP      string
	EgressCountry string
	LatencyMS     int64
}

func ParseProbeResponse(raw []byte) ProbeResponse {
	var body struct {
		IP      string `json:"ip"`
		Country string `json:"country"`
		Loc     string `json:"loc"`
	}
	if err := json.Unmarshal(raw, &body); err == nil {
		return ProbeResponse{
			EgressIP:     strings.TrimSpace(body.IP),
			ProbeCountry: strings.ToUpper(strings.TrimSpace(firstNonEmpty(body.Country, body.Loc))),
		}
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return ProbeResponse{
		EgressIP:     strings.TrimSpace(values["ip"]),
		ProbeCountry: strings.ToUpper(strings.TrimSpace(firstNonEmpty(values["loc"], values["country"]))),
	}
}

func BuildSuccessRecord(response ProbeResponse, geoIPCountry string, latencyMS int64) SuccessRecord {
	country := strings.ToUpper(strings.TrimSpace(geoIPCountry))
	if country == "" {
		country = response.ProbeCountry
	}
	return SuccessRecord{
		EgressIP:      response.EgressIP,
		EgressCountry: country,
		LatencyMS:     latencyMS,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
