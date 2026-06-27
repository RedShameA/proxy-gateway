package httpapi

import (
	"io"
	"net/http"
)

type RunEvaluationsHandler struct {
	Auth AuthFunc
	Run  func(forceSwitch bool) (evaluated int, skipped int, err error)
}

func (h RunEvaluationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ForceSwitch bool `json:"force_switch"`
	}
	if err := readJSON(r, &req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	evaluated, skipped, err := h.Run(req.ForceSwitch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"evaluated_profiles": evaluated,
		"skipped_profiles":   skipped,
	})
}
