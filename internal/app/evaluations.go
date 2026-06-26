package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	appevaluations "proxygateway/internal/application/evaluations"
	domainprofile "proxygateway/internal/domain/profile"
)

const defaultProfileTestURL = "https://www.gstatic.com/generate_204"

func (g *Gateway) handleRunEvaluations(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(w, r) {
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
	settings, err := g.loadEvaluationSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load evaluation settings")
		return
	}
	maintenanceSettings, err := g.loadMaintenanceSettings()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load maintenance settings")
		return
	}
	rows, err := g.db.Query(`SELECT id, type, fixed_node_id, exit_node_ids_json, chain_evaluation_mode, test_url,
		       egress_country, egress_country_mode, egress_countries_json, node_source_mode, source_ids_json, protocols_json,
		       name_include_regex, name_exclude_regex, manual_only, candidate_limit, min_evaluation_interval_seconds,
		       last_evaluated_at, config_version, relative_improvement_threshold, absolute_latency_improvement_ms,
		       node_sticky_enabled
		  FROM access_profiles
		 WHERE type IN ('fastest', 'chain')
		 ORDER BY created_at, id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list profiles")
		return
	}
	var targets []profileEvaluationTarget
	now := unixMillisNow()
	skipped := 0
	for rows.Next() {
		var profileID, profileType, fixedNodeID, exitNodeIDsJSON, chainEvaluationMode, testURL, egressCountry, egressCountryMode, egressCountriesJSON, nodeSourceMode, sourceIDsJSON, protocolsJSON, includeRegex, excludeRegex string
		var manualOnly, candidateLimit, minInterval, absoluteImprovementMS, nodeStickyEnabled int
		var relativeImprovementThreshold float64
		var lastEvaluatedAt, configVersion int64
		if err := rows.Scan(&profileID, &profileType, &fixedNodeID, &exitNodeIDsJSON, &chainEvaluationMode, &testURL, &egressCountry, &egressCountryMode, &egressCountriesJSON, &nodeSourceMode, &sourceIDsJSON, &protocolsJSON, &includeRegex, &excludeRegex, &manualOnly, &candidateLimit, &minInterval, &lastEvaluatedAt, &configVersion, &relativeImprovementThreshold, &absoluteImprovementMS, &nodeStickyEnabled); err != nil {
			continue
		}
		snapshot, shouldSkip := appevaluations.BuildTargetSnapshot(appevaluations.TargetSnapshotInput{
			FixedNodeID:                         fixedNodeID,
			ExitNodeIDs:                         unmarshalStringSlice(exitNodeIDsJSON),
			TestURL:                             testURL,
			DefaultTestURL:                      defaultProfileTestURL,
			LastEvaluatedAt:                     lastEvaluatedAt,
			MinEvaluationIntervalSeconds:        minInterval,
			DefaultMinEvaluationIntervalSeconds: settings.DefaultMinEvaluationIntervalSeconds,
			NowMS:                               now,
			ForceSwitch:                         req.ForceSwitch,
		})
		if shouldSkip {
			skipped++
			continue
		}
		filter := candidateFilterFromFields(egressCountry, egressCountryMode, egressCountriesJSON, nodeSourceMode, sourceIDsJSON, protocolsJSON, includeRegex, excludeRegex, manualOnly)
		targets = append(targets, profileEvaluationTarget{
			ID:                           profileID,
			Type:                         profileType,
			FixedNodeID:                  fixedNodeID,
			ExitNodeIDs:                  snapshot.ExitNodeIDs,
			ChainEvaluationMode:          normalizeChainEvaluationMode(chainEvaluationMode),
			TestURL:                      snapshot.TestURL,
			Filter:                       filter,
			CandidateLimit:               candidateLimit,
			MinEvaluationIntervalSeconds: minInterval,
			RelativeImprovementThreshold: relativeImprovementThreshold,
			AbsoluteImprovementMS:        absoluteImprovementMS,
			LastEvaluatedAt:              lastEvaluatedAt,
			ConfigVersion:                configVersion,
			ForceSwitch:                  req.ForceSwitch,
			NodeStickyEnabled:            nodeStickyEnabled == 1,
		})
	}
	if err := rows.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "close profile rows")
		return
	}
	evaluated := g.runProfileEvaluations(targets, settings, maintenanceSettings.ProfileEvaluationConcurrency)
	writeJSON(w, http.StatusOK, map[string]int{
		"evaluated_profiles": evaluated,
		"skipped_profiles":   skipped,
	})
}

func (g *Gateway) runProfileEvaluations(targets []profileEvaluationTarget, settings evaluationSettings, profileConcurrency int) int {
	if len(targets) == 0 {
		return 0
	}
	if profileConcurrency <= 0 {
		profileConcurrency = 1
	}
	if profileConcurrency > len(targets) {
		profileConcurrency = len(targets)
	}
	sem := make(chan struct{}, profileConcurrency)
	results := make(chan bool, len(targets))
	var wg sync.WaitGroup
	for _, target := range targets {
		target := target
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results <- g.runOneProfileEvaluation(target, settings)
		}()
	}
	wg.Wait()
	close(results)
	evaluated := 0
	for range results {
		evaluated++
	}
	return evaluated
}

func (g *Gateway) profileEvaluationTarget(profileID string, forceSwitch bool) (profileEvaluationTarget, evaluationSettings, bool, error) {
	settings, err := g.loadEvaluationSettings()
	if err != nil {
		return profileEvaluationTarget{}, evaluationSettings{}, false, err
	}
	var profileType, fixedNodeID, exitNodeIDsJSON, chainEvaluationMode, testURL, egressCountry, egressCountryMode, egressCountriesJSON, nodeSourceMode, sourceIDsJSON, protocolsJSON, includeRegex, excludeRegex string
	var manualOnly, candidateLimit, minInterval, absoluteImprovementMS, nodeStickyEnabled int
	var relativeImprovementThreshold float64
	var lastEvaluatedAt, configVersion int64
	err = g.db.QueryRow(
		`SELECT type, fixed_node_id, exit_node_ids_json, chain_evaluation_mode, test_url,
		        egress_country, egress_country_mode, egress_countries_json, node_source_mode, source_ids_json, protocols_json,
		        name_include_regex, name_exclude_regex, manual_only, candidate_limit, min_evaluation_interval_seconds,
		        last_evaluated_at, config_version, relative_improvement_threshold, absolute_latency_improvement_ms,
		        node_sticky_enabled
		   FROM access_profiles WHERE id = ?`,
		profileID,
	).Scan(&profileType, &fixedNodeID, &exitNodeIDsJSON, &chainEvaluationMode, &testURL, &egressCountry, &egressCountryMode, &egressCountriesJSON, &nodeSourceMode, &sourceIDsJSON, &protocolsJSON, &includeRegex, &excludeRegex, &manualOnly, &candidateLimit, &minInterval, &lastEvaluatedAt, &configVersion, &relativeImprovementThreshold, &absoluteImprovementMS, &nodeStickyEnabled)
	if err != nil {
		return profileEvaluationTarget{}, settings, false, err
	}
	if !profileTypeNeedsEvaluation(profileType) {
		return profileEvaluationTarget{}, settings, true, nil
	}
	snapshot, shouldSkip := appevaluations.BuildTargetSnapshot(appevaluations.TargetSnapshotInput{
		FixedNodeID:                         fixedNodeID,
		ExitNodeIDs:                         unmarshalStringSlice(exitNodeIDsJSON),
		TestURL:                             testURL,
		DefaultTestURL:                      defaultProfileTestURL,
		LastEvaluatedAt:                     lastEvaluatedAt,
		MinEvaluationIntervalSeconds:        minInterval,
		DefaultMinEvaluationIntervalSeconds: settings.DefaultMinEvaluationIntervalSeconds,
		NowMS:                               unixMillisNow(),
		ForceSwitch:                         forceSwitch,
	})
	if shouldSkip {
		return profileEvaluationTarget{}, settings, true, nil
	}
	return profileEvaluationTarget{
		ID:                           profileID,
		Type:                         profileType,
		FixedNodeID:                  fixedNodeID,
		ExitNodeIDs:                  snapshot.ExitNodeIDs,
		ChainEvaluationMode:          normalizeChainEvaluationMode(chainEvaluationMode),
		TestURL:                      snapshot.TestURL,
		Filter:                       candidateFilterFromFields(egressCountry, egressCountryMode, egressCountriesJSON, nodeSourceMode, sourceIDsJSON, protocolsJSON, includeRegex, excludeRegex, manualOnly),
		CandidateLimit:               candidateLimit,
		MinEvaluationIntervalSeconds: minInterval,
		RelativeImprovementThreshold: relativeImprovementThreshold,
		AbsoluteImprovementMS:        absoluteImprovementMS,
		LastEvaluatedAt:              lastEvaluatedAt,
		ConfigVersion:                configVersion,
		ForceSwitch:                  forceSwitch,
		NodeStickyEnabled:            nodeStickyEnabled == 1,
	}, settings, false, nil
}

func (g *Gateway) runOneProfileEvaluation(target profileEvaluationTarget, settings evaluationSettings) bool {
	switch target.Type {
	case "fastest":
		return g.evaluateFastestProfile(target, settings)
	case "chain":
		if normalizeChainEvaluationMode(target.ChainEvaluationMode) == "chain_link" {
			return g.evaluateFastestFrontProfile(target, settings)
		}
		return g.evaluateEndToEndChainProfile(target, settings)
	default:
		return false
	}
}

func profileTypeNeedsEvaluation(profileType string) bool {
	return profileType == "fastest" || profileType == "chain"
}

func (g *Gateway) profileConfigVersionMatches(profileID string, configVersion int64) bool {
	if configVersion == 0 {
		return true
	}
	current := g.profileCurrentConfigVersion(profileID)
	if current == 0 {
		return false
	}
	return current == configVersion
}

func (g *Gateway) profileCurrentConfigVersion(profileID string) int64 {
	var current int64
	_ = g.db.QueryRow(`SELECT config_version FROM access_profiles WHERE id = ?`, profileID).Scan(&current)
	return current
}

func (g *Gateway) updateProfileEvaluationState(target profileEvaluationTarget, setClause string, args ...any) bool {
	query := `UPDATE access_profiles SET ` + setClause + ` WHERE id = ? AND (config_version = ? OR ? = 0)`
	args = append(args, target.ID, target.ConfigVersion, target.ConfigVersion)
	res, err := g.db.Exec(query, args...)
	if err != nil {
		return false
	}
	affected, err := res.RowsAffected()
	return err == nil && affected > 0
}

func (g *Gateway) updateProfileEvaluationStateAndReleaseRetained(target profileEvaluationTarget, keepNodeIDs []string, setClause string, args ...any) bool {
	query := `UPDATE access_profiles SET ` + setClause + ` WHERE id = ? AND (config_version = ? OR ? = 0)`
	tx, err := g.db.Begin()
	if err != nil {
		return false
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	updateArgs := append(args, target.ID, target.ConfigVersion, target.ConfigVersion)
	res, err := tx.Exec(query, updateArgs...)
	if err != nil {
		return false
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		return false
	}
	var deletedFingerprints []string
	if target.NodeStickyEnabled {
		deletedFingerprints, err = releaseRetainedProfileNodesExceptTx(tx, target.ID, keepNodeIDs)
		if err != nil {
			return false
		}
	}
	if err := tx.Commit(); err != nil {
		return false
	}
	committed = true
	g.invalidateRuntimeFingerprints(deletedFingerprints)
	return true
}

func (g *Gateway) profileLastError(profileID string) string {
	var lastError string
	_ = g.db.QueryRow(`SELECT last_error FROM access_profiles WHERE id = ?`, profileID).Scan(&lastError)
	return lastError
}

func effectiveProfileTestURL(testURL string) string {
	testURL = strings.TrimSpace(testURL)
	if testURL == "" {
		return defaultProfileTestURL
	}
	if !strings.Contains(testURL, "://") {
		return "https://" + testURL
	}
	return testURL
}

func validateProfileTestURL(testURL string) error {
	effective := effectiveProfileTestURL(testURL)
	u, err := url.Parse(effective)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New(validationTestURLScheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return errors.New(validationTestURLHostRequired)
	}
	return nil
}

type profileCandidateProbeResult struct {
	Node     nodeRecord
	Duration int64
	Status   int
	Err      error
}

func (r profileCandidateProbeResult) succeeded() bool {
	return r.Err == nil
}

func (r profileCandidateProbeResult) failureMessage() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	return "test url probe failed without error detail"
}

func (g *Gateway) evaluateCandidateProbes(nodes []nodeRecord, concurrency int, probe func(nodeRecord) profileCandidateProbeResult) []profileCandidateProbeResult {
	if len(nodes) == 0 {
		return nil
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(nodes) {
		concurrency = len(nodes)
	}
	jobs := make(chan nodeRecord)
	results := make(chan profileCandidateProbeResult, len(nodes))
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for node := range jobs {
				results <- probe(node)
			}
		}()
	}
	go func() {
		for _, node := range nodes {
			jobs <- node
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	collected := make([]profileCandidateProbeResult, 0, len(nodes))
	for result := range results {
		collected = append(collected, result)
	}
	return collected
}

func (g *Gateway) evaluateFastestProfile(target profileEvaluationTarget, settings evaluationSettings) bool {
	startedAt := unixMillisNow()
	if !g.updateProfileEvaluationState(target, `state = 'running', last_error = '', last_evaluation_started_at = ?`, startedAt) {
		return false
	}
	nodes, err := g.candidateNodes(target.Filter)
	if err != nil || len(nodes) == 0 {
		lastError := "no candidate nodes"
		if err != nil {
			lastError = err.Error()
		}
		currentNodeID := g.profileCurrentNodeID(target.ID)
		outcome := appevaluations.PlanFastestNoCandidate(lastError, target.NodeStickyEnabled && g.profileRetainsNode(target.ID, currentNodeID))
		if outcome.IncrementCurrentPathCounters {
			g.updateProfileEvaluationState(target, `state = ?, last_error = ?, switch_reason = ?, current_path_failed_evaluations = current_path_failed_evaluations + 1, current_path_missed_success_cycles = current_path_missed_success_cycles + 1, last_evaluated_at = ?`, outcome.State, outcome.LastError, outcome.SwitchReason, unixMillisNow())
			return false
		}
		g.updateProfileEvaluationState(target, `state = ?, current_node_id = '', last_error = ?, switch_reason = ?, last_evaluated_at = ?`, outcome.State, outcome.LastError, outcome.SwitchReason, unixMillisNow())
		return false
	}
	nodes = limitNodes(nodes, g.effectiveCandidateLimit(target.CandidateLimit, settings.SingleCandidateLimit))
	currentNodeID := g.profileCurrentNodeID(target.ID)
	if currentNodeID != "" && !nodeIDInRecords(currentNodeID, nodes) {
		if !g.profileRetainsNode(target.ID, currentNodeID) {
			currentNodeID = ""
		}
	}
	results := g.evaluateCandidateProbes(
		nodes,
		settings.GlobalConcurrency,
		func(node nodeRecord) profileCandidateProbeResult {
			duration, status, err := g.fetchTestURLThroughNode(node, target.TestURL, settings)
			return profileCandidateProbeResult{Node: node, Duration: duration, Status: status, Err: err}
		},
	)
	probeResults := make([]appevaluations.FastestProbeResult, 0, len(results))
	for _, result := range results {
		probeResults = append(probeResults, appevaluations.FastestProbeResult{
			NodeID:     result.Node.ID,
			DurationMS: result.Duration,
			OK:         result.succeeded(),
			Error:      result.failureMessage(),
		})
	}
	summary := appevaluations.SummarizeFastestProbeResults(currentNodeID, probeResults)
	finishedAt := unixMillisNow()
	if summary.BestNodeID == "" {
		outcome := appevaluations.PlanFastestAllCandidatesFailed(currentNodeID, summary.LastFailure)
		if outcome.IncrementCurrentPathCounters {
			g.updateProfileEvaluationState(target, `state = ?, last_error = ?, switch_reason = ?, current_path_failed_evaluations = current_path_failed_evaluations + 1, current_path_missed_success_cycles = current_path_missed_success_cycles + 1, last_evaluated_at = ?`, outcome.State, outcome.LastError, outcome.SwitchReason, finishedAt)
			return false
		}
		g.updateProfileEvaluationState(target, `state = ?, current_node_id = '', last_error = ?, switch_reason = ?, last_evaluated_at = ?`, outcome.State, outcome.LastError, outcome.SwitchReason, finishedAt)
		return false
	}
	selectedNodeID, selectedDuration, state, reason, failedCount, missedCycles := g.stableProfileSelection(target, currentNodeID, summary.CurrentDurationMS, summary.BestNodeID, summary.BestDurationMS)
	details := appevaluations.BuildFastestDetails(appevaluations.FastestDetailsInput{
		TestURL:           target.TestURL,
		CandidateCount:    len(nodes),
		FailureCount:      summary.FailureCount,
		BestNodeID:        summary.BestNodeID,
		BestDurationMS:    summary.BestDurationMS,
		CurrentNodeID:     currentNodeID,
		CurrentDurationMS: summary.CurrentDurationMS,
		SelectedNodeID:    selectedNodeID,
		SwitchReason:      reason,
	})
	if !g.selectedProfileNodeStillValid(target, selectedNodeID) {
		finishedAt = unixMillisNow()
		details["selected_node_id"] = selectedNodeID
		details["switch_reason"] = "selected_node_removed"
		details["reason"] = "selected_node_removed"
		detailsJSON, _ := json.Marshal(details)
		g.updateProfileEvaluationState(
			target,
			`state = 'waiting_observation',
			 current_node_id = '',
			 last_error = 'selected node no longer exists',
			 current_path_latency_ms = 0,
			 current_path_failed_evaluations = current_path_failed_evaluations + 1,
			 current_path_missed_success_cycles = current_path_missed_success_cycles + 1,
			 switch_reason = 'selected_node_removed',
			 last_evaluation_details_json = ?,
			 last_evaluated_at = ?`,
			string(detailsJSON),
			finishedAt,
		)
		return false
	}
	detailsJSON, _ := json.Marshal(details)
	return g.updateProfileEvaluationStateAndReleaseRetained(
		target,
		[]string{selectedNodeID},
		`state = ?,
		 current_node_id = ?,
		 last_error = '',
		 current_path_latency_ms = ?,
		 current_path_failed_evaluations = ?,
		 current_path_missed_success_cycles = ?,
		 switch_reason = ?,
		 last_evaluation_details_json = ?,
		 last_evaluated_at = ?`,
		state,
		selectedNodeID,
		selectedDuration,
		failedCount,
		missedCycles,
		reason,
		string(detailsJSON),
		finishedAt,
	)
}

func (g *Gateway) stableProfileSelection(target profileEvaluationTarget, currentNodeID string, currentDuration int64, bestNodeID string, bestDuration int64) (string, int64, string, string, int, int) {
	selection := appevaluations.SelectFastestPath(appevaluations.FastestSelectionInput{
		CurrentNodeID:                currentNodeID,
		CurrentDurationMS:            currentDuration,
		BestNodeID:                   bestNodeID,
		BestDurationMS:               bestDuration,
		RelativeImprovementThreshold: target.RelativeImprovementThreshold,
		AbsoluteLatencyImprovementMS: target.AbsoluteImprovementMS,
		ForceSwitch:                  target.ForceSwitch,
	})
	return selection.SelectedNodeID, selection.SelectedDurationMS, selection.State, selection.SwitchReason, selection.FailedCount, selection.MissedSuccessCycles
}

func (g *Gateway) selectedProfileNodeStillValid(target profileEvaluationTarget, nodeID string) bool {
	if nodeID == "" {
		return false
	}
	node, err := g.loadNode(nodeID)
	if err != nil || !node.Enabled {
		return false
	}
	if g.profileRetainsNode(target.ID, nodeID) {
		return true
	}
	return g.nodeMatchesCandidateFilter(node, target.Filter)
}

func nodeIDInRecords(nodeID string, nodes []nodeRecord) bool {
	if nodeID == "" {
		return false
	}
	for _, node := range nodes {
		if node.ID == nodeID {
			return true
		}
	}
	return false
}

func (g *Gateway) profileCurrentPathCounters(profileID string) (int, int) {
	var failures, missed int
	_ = g.db.QueryRow(`SELECT current_path_failed_evaluations, current_path_missed_success_cycles FROM access_profiles WHERE id = ?`, profileID).Scan(&failures, &missed)
	return failures, missed
}

func (g *Gateway) profileCurrentPathLatency(profileID string) int64 {
	var latency int64
	_ = g.db.QueryRow(`SELECT current_path_latency_ms FROM access_profiles WHERE id = ?`, profileID).Scan(&latency)
	return latency
}

func (g *Gateway) effectiveCandidateLimit(profileLimit, settingsLimit int) int {
	if profileLimit > 0 {
		return profileLimit
	}
	return settingsLimit
}

func limitNodes(nodes []nodeRecord, limit int) []nodeRecord {
	if limit <= 0 || len(nodes) <= limit {
		return nodes
	}
	return nodes[:limit]
}

func (g *Gateway) profileCurrentNodeID(profileID string) string {
	var nodeID string
	_ = g.db.QueryRow(`SELECT current_node_id FROM access_profiles WHERE id = ?`, profileID).Scan(&nodeID)
	return nodeID
}

func candidateFilterFromFields(egressCountry, egressCountryMode, egressCountriesJSON, nodeSourceMode, sourceIDsJSON, protocolsJSON, includeRegex, excludeRegex string, manualOnly int) candidateFilter {
	filter := candidateFilter{
		EgressCountry:     egressCountry,
		EgressCountries:   unmarshalStringSlice(egressCountriesJSON),
		EgressCountryMode: egressCountryMode,
		NodeSourceMode:    nodeSourceMode,
		SourceIDs:         unmarshalStringSlice(sourceIDsJSON),
		Protocols:         unmarshalStringSlice(protocolsJSON),
		NameIncludeRegex:  includeRegex,
		NameExcludeRegex:  excludeRegex,
		ManualOnly:        manualOnly == 1,
	}
	return candidateFilterFromDomain(domainprofile.NormalizeCandidateFilter(filter.domainFilter()))
}

func (g *Gateway) candidateNodes(filter candidateFilter) ([]nodeRecord, error) {
	filter = candidateFilterFromDomain(domainprofile.NormalizeCandidateFilter(filter.domainFilter()))
	domainFilter := filter.domainFilter()
	rows, err := g.db.Query(
		`SELECT n.id
		   FROM nodes n
		  WHERE n.enabled = 1
		    AND NOT (
		      NOT EXISTS (SELECT 1 FROM node_sources s WHERE s.node_id = n.id)
		      AND EXISTS (SELECT 1 FROM retained_profile_nodes r WHERE r.node_id = n.id)
		    )
		  ORDER BY n.created_at, n.rowid`,
	)
	if err != nil {
		return nil, err
	}
	var nodeIDs []string
	for rows.Next() {
		var nodeID string
		if err := rows.Scan(&nodeID); err != nil {
			continue
		}
		nodeIDs = append(nodeIDs, nodeID)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var nodes []nodeRecord
	for _, nodeID := range nodeIDs {
		if len(filter.EgressCountries) > 0 {
			var country string
			err := g.db.QueryRow(`SELECT egress_country FROM node_observations WHERE node_id = ? AND usable = 1`, nodeID).Scan(&country)
			if err != nil || strings.TrimSpace(country) == "" {
				country = ""
			}
			if !domainprofile.MatchEgressCountry(domainFilter, country) {
				continue
			}
		}
		node, err := g.loadNode(nodeID)
		if err == nil && g.nodeMatchesCandidateFilter(node, filter) {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func (g *Gateway) nodeMatchesCandidateFilter(node nodeRecord, filter candidateFilter) bool {
	return domainprofile.MatchCandidateNode(filter.domainFilter(), g.loadCandidateNode(node, filter))
}

func normalizeNodeSourceMode(mode string, sourceIDs []string, manualOnly bool) string {
	return domainprofile.NormalizeNodeSourceMode(mode, sourceIDs, manualOnly)
}

func internalNodeSourceMode(mode string) string {
	return normalizeNodeSourceMode(mode, nil, false)
}

func apiNodeSourceMode(mode string) string {
	switch domainprofile.NormalizeNodeSourceMode(mode, nil, false) {
	case "subscriptions":
		return "subscription"
	case "specific_subscriptions":
		return "selected_sources"
	case "manual":
		return "manual"
	default:
		return "all"
	}
}

func (filter candidateFilter) domainFilter() domainprofile.CandidateFilter {
	return domainprofile.CandidateFilter{
		EgressCountry:     filter.EgressCountry,
		EgressCountries:   filter.EgressCountries,
		EgressCountryMode: filter.EgressCountryMode,
		NodeSourceMode:    filter.NodeSourceMode,
		SourceIDs:         filter.SourceIDs,
		Protocols:         filter.Protocols,
		NameIncludeRegex:  filter.NameIncludeRegex,
		NameExcludeRegex:  filter.NameExcludeRegex,
		ManualOnly:        filter.ManualOnly,
	}
}

func candidateFilterFromDomain(filter domainprofile.CandidateFilter) candidateFilter {
	return candidateFilter{
		EgressCountry:     filter.EgressCountry,
		EgressCountries:   filter.EgressCountries,
		EgressCountryMode: filter.EgressCountryMode,
		NodeSourceMode:    filter.NodeSourceMode,
		SourceIDs:         filter.SourceIDs,
		Protocols:         filter.Protocols,
		NameIncludeRegex:  filter.NameIncludeRegex,
		NameExcludeRegex:  filter.NameExcludeRegex,
		ManualOnly:        filter.ManualOnly,
	}
}

func (g *Gateway) loadCandidateNode(node nodeRecord, filter candidateFilter) domainprofile.CandidateNode {
	candidate := domainprofile.CandidateNode{
		Type: node.Type,
		Name: node.Name,
	}
	if filter.NodeSourceMode == "all" && !filter.ManualOnly && len(filter.SourceIDs) == 0 {
		return candidate
	}
	rows, err := g.db.Query(`SELECT source_type, source_id FROM node_sources WHERE node_id = ?`, node.ID)
	if err != nil {
		return candidate
	}
	defer rows.Close()
	for rows.Next() {
		var sourceType, sourceID string
		if err := rows.Scan(&sourceType, &sourceID); err != nil {
			continue
		}
		candidate.SourceTypes = append(candidate.SourceTypes, sourceType)
		candidate.SourceIDs = append(candidate.SourceIDs, sourceID)
	}
	return candidate
}

func (g *Gateway) fetchTestURLThroughNode(node nodeRecord, testURL string, settings evaluationSettings) (int64, int, error) {
	outbound, err := buildOutboundGETRequest(testURL)
	if err != nil {
		return 0, 0, err
	}
	start := time.Now()
	timeouts := settings.probeDialTimeouts()
	conn, err := g.dialViaNode(node, outbound.TargetHost, timeouts)
	if err != nil {
		return 0, 0, err
	}
	defer conn.Close()
	if !timeouts.Deadline.IsZero() {
		deadlineConn := conn
		_ = deadlineConn.SetDeadline(timeouts.Deadline)
		defer func() { _ = deadlineConn.SetDeadline(time.Time{}) }()
	}
	conn, err = wrapOutboundGETConn(conn, outbound)
	if err != nil {
		return 0, 0, err
	}
	if err := outbound.Request.Write(conn); err != nil {
		return 0, 0, err
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), outbound.Request)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
	return time.Since(start).Milliseconds(), resp.StatusCode, nil
}
