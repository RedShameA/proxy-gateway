package app

import (
	"errors"
	"net/http"
	"time"
)

const (
	defaultConnectTimeoutSeconds = 10
	defaultProbeTimeoutSeconds   = 10
)

func (g *Gateway) handleEvaluationSettings(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := g.loadEvaluationSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load evaluation settings")
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPost, http.MethodPut:
		var req evaluationSettings
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req = normalizeEvaluationSettings(req)
		if err := validateEvaluationSettings(req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := g.saveEvaluationSettings(req); err != nil {
			writeError(w, http.StatusInternalServerError, "save evaluation settings")
			return
		}
		writeJSON(w, http.StatusOK, req)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (g *Gateway) loadEvaluationSettings() (evaluationSettings, error) {
	var settings evaluationSettings
	err := g.db.QueryRow(
		`SELECT global_concurrency,
		        default_min_evaluation_interval_seconds,
		        single_candidate_limit,
		        chain_candidate_limit,
		        connect_timeout_seconds,
		        probe_timeout_seconds
		   FROM evaluation_settings
		  WHERE id = 1`,
	).Scan(
		&settings.GlobalConcurrency,
		&settings.DefaultMinEvaluationIntervalSeconds,
		&settings.SingleCandidateLimit,
		&settings.ChainCandidateLimit,
		&settings.ConnectTimeoutSeconds,
		&settings.ProbeTimeoutSeconds,
	)
	return normalizeEvaluationSettings(settings), err
}

func normalizeEvaluationSettings(settings evaluationSettings) evaluationSettings {
	if settings.GlobalConcurrency <= 0 {
		settings.GlobalConcurrency = 1
	}
	if settings.ConnectTimeoutSeconds <= 0 {
		settings.ConnectTimeoutSeconds = defaultConnectTimeoutSeconds
	}
	if settings.ProbeTimeoutSeconds <= 0 {
		settings.ProbeTimeoutSeconds = defaultProbeTimeoutSeconds
	}
	return settings
}

func validateEvaluationSettings(settings evaluationSettings) error {
	if settings.GlobalConcurrency <= 0 ||
		settings.DefaultMinEvaluationIntervalSeconds < 0 ||
		settings.SingleCandidateLimit < 0 ||
		settings.ChainCandidateLimit < 0 ||
		settings.ConnectTimeoutSeconds <= 0 ||
		settings.ProbeTimeoutSeconds <= 0 {
		return errors.New(validationEvaluationSettingsRange)
	}
	return nil
}

func (g *Gateway) saveEvaluationSettings(settings evaluationSettings) error {
	_, err := g.db.Exec(
		`UPDATE evaluation_settings
		 SET global_concurrency = ?,
		     default_min_evaluation_interval_seconds = ?,
		     single_candidate_limit = ?,
		     chain_candidate_limit = ?,
		     connect_timeout_seconds = ?,
		     probe_timeout_seconds = ?
		 WHERE id = 1`,
		settings.GlobalConcurrency,
		settings.DefaultMinEvaluationIntervalSeconds,
		settings.SingleCandidateLimit,
		settings.ChainCandidateLimit,
		settings.ConnectTimeoutSeconds,
		settings.ProbeTimeoutSeconds,
	)
	return err
}

func (settings evaluationSettings) probeDialTimeouts() dialTimeouts {
	settings = normalizeEvaluationSettings(settings)
	return dialTimeouts{
		ConnectTimeout: time.Duration(settings.ConnectTimeoutSeconds) * time.Second,
		Deadline:       time.Now().Add(time.Duration(settings.ProbeTimeoutSeconds) * time.Second),
	}
}
