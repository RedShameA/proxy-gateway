package httpapi

import (
	"net/http"

	appdictionaries "proxygateway/internal/application/dictionaries"
)

type EgressCountriesHandler struct {
	Auth AuthFunc
	Repo appdictionaries.Repository
}

func (h EgressCountriesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	countryCodes, err := h.Repo.ListEgressCountries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query countries")
		return
	}
	writeJSON(w, http.StatusOK, egressCountryEntries(countryCodes))
}

type egressCountryEntry struct {
	Value     string  `json:"value"`
	ISOCode   *string `json:"iso_code"`
	NameZH    string  `json:"name_zh"`
	IsUnknown bool    `json:"is_unknown"`
}

func egressCountryEntries(countryCodes []string) []egressCountryEntry {
	countries := []egressCountryEntry{{
		Value:     "__unknown__",
		ISOCode:   nil,
		NameZH:    "未知",
		IsUnknown: true,
	}}
	for _, code := range countryCodes {
		iso := code
		countries = append(countries, egressCountryEntry{
			Value:     code,
			ISOCode:   &iso,
			NameZH:    appdictionaries.CountryNameZH(code),
			IsUnknown: false,
		})
	}
	return countries
}
