package app

import (
	"net/http"
)

func (g *Gateway) handleOverview(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Resource counts
	var counts struct {
		Subscriptions int `json:"subscriptions"`
		Nodes         int `json:"nodes"`
		UsableNodes   int `json:"usable_nodes"`
		Profiles      int `json:"access_profiles"`
		Credentials   int `json:"proxy_credentials"`
		Requests24h   int `json:"requests_24h"`
		Failed24h     int `json:"failed_requests_24h"`
	}
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM subscriptions`).Scan(&counts.Subscriptions)
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&counts.Nodes)
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM nodes n JOIN node_observations o ON n.id = o.node_id WHERE o.usable = 1 AND n.enabled = 1`).Scan(&counts.UsableNodes)
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM access_profiles`).Scan(&counts.Profiles)
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM proxy_credentials`).Scan(&counts.Credentials)
	requestLogCutoff := unixMillisNow() - secondsToMillis(86400)
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE ts > ?`, requestLogCutoff).Scan(&counts.Requests24h)
	_ = g.db.QueryRow(`SELECT COUNT(*) FROM request_logs WHERE ts > ? AND state = 'completed' AND success = 0`, requestLogCutoff).Scan(&counts.Failed24h)

	// Profile state distribution
	stateCounts := map[string]int{
		"pending": 0, "running": 0, "waiting_observation": 0, "ready": 0, "degraded": 0,
		"no_candidate": 0, "failed": 0, "invalid_config": 0,
	}
	rows, err := g.db.Query(`SELECT state, COUNT(*) FROM access_profiles GROUP BY state`)
	if err == nil {
		for rows.Next() {
			var state string
			var count int
			if err := rows.Scan(&state, &count); err == nil {
				stateCounts[state] = count
			}
		}
		_ = rows.Close()
	}

	// Recent failures (last 10)
	var failures []map[string]any
	fRows, err := g.db.Query(`SELECT id, ts, target_host, error, access_profile_id, access_profile, access_profile_identifier,
	                                 proxy_credential_id, proxy_credential, proxy_path_json, failure_stage, duration_ms, ingress_bytes, egress_bytes, http_status
	                            FROM request_logs WHERE state = 'completed' AND success = 0 ORDER BY ts DESC LIMIT 10`)
	if err == nil {
		for fRows.Next() {
			var fid, target, errText, profileID, profileName, profileIdentifier, credID, credName, proxyPathJSON, failureStage string
			var fts int64
			var durationMS, ingressBytes, egressBytes int64
			var httpStatus int
			if err := fRows.Scan(&fid, &fts, &target, &errText, &profileID, &profileName, &profileIdentifier, &credID, &credName, &proxyPathJSON, &failureStage, &durationMS, &ingressBytes, &egressBytes, &httpStatus); err == nil {
				var credentialID any = credID
				if credID == "" {
					credentialID = nil
				}
				var httpStatusPtr any = nil
				if httpStatus > 0 {
					httpStatusPtr = httpStatus
				}
				failures = append(failures, map[string]any{
					"id": fid, "occurred_at": fts, "target": target, "target_host": target, "target_port": targetPortFromTarget(target),
					"error": errText, "result": "failure", "failure_stage": failureStage, "duration_ms": durationMS,
					"state": "completed", "success": false,
					"ingress_bytes": ingressBytes, "egress_bytes": egressBytes, "http_status": httpStatusPtr,
					"proxy_path":       parseRequestLogProxyPath(proxyPathJSON),
					"access_profile":   map[string]any{"id": profileID, "name": profileName, "profile_identifier": profileIdentifier},
					"proxy_credential": map[string]any{"id": credentialID, "remark": credName},
				})
			}
		}
		_ = fRows.Close()
	}
	if failures == nil {
		failures = []map[string]any{}
	}

	// Maintenance runs (last 10)
	maintenanceRuns := []map[string]any{}
	runRows, err := g.db.Query(
		`SELECT id, run_type, trigger_source, target_id, target_label, state, result, reason_code,
		        total_count, finished_count, detail_json, last_error, created_at, started_at, finished_at, updated_at
		   FROM maintenance_runs
		  ORDER BY created_at DESC, rowid DESC
		  LIMIT 10`,
	)
	if err == nil {
		for runRows.Next() {
			run, err := scanMaintenanceRun(runRows)
			if err == nil {
				maintenanceRuns = append(maintenanceRuns, run.toMap())
			}
		}
		_ = runRows.Close()
	}

	// Access profiles summary
	profiles := g.listAccessProfilesSummary()

	// GeoIP status
	geoip := g.geoIPStatus()

	writeJSON(w, http.StatusOK, map[string]any{
		"resource_counts":      counts,
		"profile_state_counts": stateCounts,
		"access_profiles":      profiles,
		"recent_failures":      failures,
		"maintenance_runs":     maintenanceRuns,
		"geoip_status":         geoip,
	})
}
