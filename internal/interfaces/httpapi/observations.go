package httpapi

import (
	"io"
	"net/http"
)

type RunNodeObservationsHandler struct {
	Auth AuthFunc
	Run  func(NodeObservationRunRequest) (NodeObservationRunResult, error)
}

type NodeObservationRunRequest struct {
	TestURL  string   `json:"test_url"`
	ProbeURL string   `json:"probe_url"`
	NodeID   string   `json:"node_id"`
	NodeIDs  []string `json:"node_ids"`
}

type NodeObservationRunResult struct {
	ObservedNodes int
	RunID         string
}

func (h RunNodeObservationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req NodeObservationRunRequest
	if err := readJSON(r, &req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	result, err := h.Run(req)
	if err != nil {
		writeStatusError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"observed_nodes": result.ObservedNodes, "run_id": result.RunID})
}
