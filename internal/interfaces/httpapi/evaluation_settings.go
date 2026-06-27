package httpapi

import (
	"errors"
	"net/http"

	appsettings "proxygateway/internal/application/settings"
)

type EvaluationSettingsHandler struct {
	Auth AuthFunc
	Repo appsettings.SystemRepository
}

func (h EvaluationSettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := h.Repo.LoadEvaluation(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load evaluation settings")
			return
		}
		writeJSON(w, http.StatusOK, appsettings.NormalizeEvaluation(settings))
	case http.MethodPost, http.MethodPut:
		var req appsettings.EvaluationSettings
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req = appsettings.NormalizeEvaluation(req)
		if err := appsettings.ValidateEvaluation(req); err != nil {
			if errors.Is(err, appsettings.ErrEvaluationSettingsRange) {
				writeError(w, http.StatusBadRequest, appsettings.EvaluationSettingsRangeError)
				return
			}
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.Repo.SaveEvaluation(r.Context(), req); err != nil {
			writeError(w, http.StatusInternalServerError, "save evaluation settings")
			return
		}
		writeJSON(w, http.StatusOK, req)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
