package httpapi

import "net/http"

type GeoIPHandler struct {
	Auth   AuthFunc
	Status func() any
	Update func() (GeoIPUpdateResult, error)
}

type GeoIPUpdateResult struct {
	RunID string
	GeoIP any
}

func (h GeoIPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"geoip": h.status()})
	case http.MethodPost:
		result, err := h.Update()
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"run_id": result.RunID, "geoip": result.GeoIP})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h GeoIPHandler) status() any {
	if h.Status == nil {
		return nil
	}
	return h.Status()
}
