package httpapi

import (
	"errors"
	"net/http"

	appsettings "proxygateway/internal/application/settings"
)

type MaintenanceSettingsHandler struct {
	Auth AuthFunc
	Repo appsettings.SystemRepository
}

func (h MaintenanceSettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := h.Repo.LoadMaintenance(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load maintenance settings")
			return
		}
		writeJSON(w, http.StatusOK, appsettings.NormalizeMaintenance(settings))
	case http.MethodPost, http.MethodPut:
		var req appsettings.MaintenanceSettings
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req = appsettings.NormalizeMaintenance(req)
		if err := appsettings.ValidateMaintenance(req); err != nil {
			writeError(w, http.StatusBadRequest, maintenanceSettingsErrorMessage(err))
			return
		}
		if err := h.Repo.SaveMaintenance(r.Context(), req); err != nil {
			writeError(w, http.StatusInternalServerError, "save maintenance settings")
			return
		}
		writeJSON(w, http.StatusOK, req)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func maintenanceSettingsErrorMessage(err error) string {
	switch {
	case errors.Is(err, appsettings.ErrMaintenanceIntervalsNonNegative):
		return appsettings.MaintenanceIntervalsNonNegativeError
	case errors.Is(err, appsettings.ErrGeoIPTimeFormat):
		return appsettings.GeoIPTimeFormatError
	case errors.Is(err, appsettings.ErrMaintenanceConcurrencyPositive):
		return appsettings.MaintenanceConcurrencyPositiveError
	default:
		return err.Error()
	}
}
