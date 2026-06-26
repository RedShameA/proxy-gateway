package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"time"

	appevaluations "proxygateway/internal/application/evaluations"
)

const defaultChainLinkProbeURL = defaultProfileTestURL

type chainCandidatePair struct {
	FrontNode nodeRecord
	ExitNode  nodeRecord
}

type chainCandidateProbeResult struct {
	Pair     chainCandidatePair
	Duration int64
	Status   int
	Err      error
}

func (r chainCandidateProbeResult) succeeded() bool {
	return r.Err == nil
}

func (r chainCandidateProbeResult) failureMessage() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	return "test url probe failed without error detail"
}

func (g *Gateway) evaluateFastestFrontProfile(target profileEvaluationTarget, settings evaluationSettings) bool {
	startedAt := unixMillisNow()
	if !g.updateProfileEvaluationState(target, `state = 'running', last_error = '', last_evaluation_started_at = ?`, startedAt) {
		return false
	}
	if len(target.ExitNodeIDs) != 1 {
		g.updateProfileEvaluationState(target, `state = 'invalid_config', current_node_id = '', current_exit_node_id = '', last_error = 'chain_link requires exactly one exit_node_id', switch_reason = 'invalid_chain_config', last_evaluated_at = ?`, unixMillisNow())
		return false
	}
	exitNode, err := g.loadUsableNode(target.ExitNodeIDs[0])
	if err != nil {
		if target.NodeStickyEnabled {
			if frontNodeID, exitNodeID := g.profileCurrentChainPath(target.ID); frontNodeID != "" && exitNodeID == target.ExitNodeIDs[0] && g.profileRetainsNode(target.ID, exitNodeID) {
				g.updateProfileEvaluationState(target, `state = 'degraded', last_error = ?, switch_reason = 'current_path_reused_after_failure', current_path_failed_evaluations = current_path_failed_evaluations + 1, current_path_missed_success_cycles = current_path_missed_success_cycles + 1, last_evaluated_at = ?`, err.Error(), unixMillisNow())
				return false
			}
		}
		g.updateProfileEvaluationState(target, `state = 'invalid_config', current_node_id = '', current_exit_node_id = '', last_error = ?, switch_reason = 'missing_exit_node', last_evaluated_at = ?`, err.Error(), unixMillisNow())
		return false
	}
	nodes, err := g.candidateNodes(target.Filter)
	if err != nil {
		g.updateProfileEvaluationState(target, `state = 'failed', current_node_id = '', current_exit_node_id = '', last_error = ?, switch_reason = 'candidate_filter_error', last_evaluated_at = ?`, err.Error(), unixMillisNow())
		return false
	}
	nodes = excludeNodes(nodes, target.ExitNodeIDs)
	if len(nodes) == 0 {
		if target.NodeStickyEnabled {
			if frontNodeID, exitNodeID := g.profileCurrentChainPath(target.ID); frontNodeID != "" && exitNodeID != "" && (g.profileRetainsNode(target.ID, frontNodeID) || g.profileRetainsNode(target.ID, exitNodeID)) {
				g.updateProfileEvaluationState(target, `state = 'degraded', last_error = 'no front node candidates', switch_reason = 'current_path_reused_after_failure', current_path_failed_evaluations = current_path_failed_evaluations + 1, current_path_missed_success_cycles = current_path_missed_success_cycles + 1, last_evaluated_at = ?`, unixMillisNow())
				return false
			}
		}
		g.updateProfileEvaluationState(target, `state = 'no_candidate', current_node_id = '', current_exit_node_id = '', last_error = 'no front node candidates', switch_reason = 'no_candidate', last_evaluated_at = ?`, unixMillisNow())
		return false
	}

	currentNodeID, currentExitNodeID := g.profileCurrentChainPath(target.ID)
	if currentExitNodeID == "" {
		currentExitNodeID = exitNode.ID
	}
	if currentExitNodeID != exitNode.ID || !nodeIDInRecords(currentNodeID, nodes) {
		if !(g.profileRetainsNode(target.ID, currentNodeID) || g.profileRetainsNode(target.ID, currentExitNodeID)) {
			currentNodeID = ""
			currentExitNodeID = ""
		}
	}
	results := g.evaluateCandidateProbes(
		nodes,
		settings.GlobalConcurrency,
		func(node nodeRecord) profileCandidateProbeResult {
			duration, err := g.probeChainLink(node, exitNode, settings)
			return profileCandidateProbeResult{Node: node, Duration: duration, Status: http.StatusOK, Err: err}
		},
	)
	probeResults := make([]appevaluations.ChainProbeResult, 0, len(results))
	for _, result := range results {
		probeResults = append(probeResults, appevaluations.ChainProbeResult{
			FrontNodeID: result.Node.ID,
			ExitNodeID:  exitNode.ID,
			DurationMS:  result.Duration,
			OK:          result.succeeded(),
			Error:       result.failureMessage(),
		})
	}
	summary := appevaluations.SummarizeChainProbeResults(currentNodeID, currentExitNodeID, probeResults)

	finishedAt := unixMillisNow()
	if summary.BestFrontNodeID == "" {
		if summary.LastFailure == "" {
			summary.LastFailure = "all front node candidates failed"
		}
		if currentNodeID != "" && currentExitNodeID != "" {
			g.updateProfileEvaluationState(target, `state = 'degraded', last_error = ?, switch_reason = 'current_path_reused_after_failure', current_path_failed_evaluations = current_path_failed_evaluations + 1, current_path_missed_success_cycles = current_path_missed_success_cycles + 1, last_evaluated_at = ?`, summary.LastFailure, finishedAt)
			return false
		}
		g.updateProfileEvaluationState(target, `state = 'failed', current_node_id = '', current_exit_node_id = '', last_error = ?, switch_reason = 'all_candidates_failed', last_evaluated_at = ?`, summary.LastFailure, finishedAt)
		return false
	}
	selectedNodeID, selectedExitNodeID, selectedDuration, state, reason, failedCount, missedCycles := g.stableChainProfileSelection(target, currentNodeID, currentExitNodeID, summary.CurrentDurationMS, summary.BestFrontNodeID, summary.BestExitNodeID, summary.BestDurationMS)
	details := chainEvaluationDetails("chain-link", len(nodes), summary.FailureCount, summary.BestFrontNodeID, summary.BestExitNodeID, summary.BestDurationMS, currentNodeID, currentExitNodeID, summary.CurrentDurationMS, selectedNodeID, selectedExitNodeID, reason)
	if !g.selectedChainPathStillValid(target, selectedNodeID, selectedExitNodeID) {
		finishedAt = unixMillisNow()
		details["selected_node_id"] = selectedNodeID
		details["selected_exit_node_id"] = selectedExitNodeID
		details["switch_reason"] = "selected_node_removed"
		details["reason"] = "selected_node_removed"
		detailsJSON, _ := json.Marshal(details)
		g.updateProfileEvaluationState(
			target,
			`state = 'waiting_observation',
			 current_node_id = '',
			 current_exit_node_id = '',
			 last_error = 'selected chain path no longer exists',
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
		[]string{selectedNodeID, selectedExitNodeID},
		`state = ?,
		 current_node_id = ?,
		 current_exit_node_id = ?,
		 last_error = '',
		 current_path_latency_ms = ?,
		 current_path_failed_evaluations = ?,
		 current_path_missed_success_cycles = ?,
		 switch_reason = ?,
		 last_evaluation_details_json = ?,
		 last_evaluated_at = ?`,
		state,
		selectedNodeID,
		selectedExitNodeID,
		selectedDuration,
		failedCount,
		missedCycles,
		reason,
		string(detailsJSON),
		finishedAt,
	)
}

func (g *Gateway) probeChainLink(frontNode, exitNode nodeRecord, settings evaluationSettings) (int64, error) {
	outbound, err := buildOutboundGETRequest(defaultChainLinkProbeURL)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	conn, err := g.dialViaChain(frontNode, exitNode, outbound.TargetHost, settings.probeDialTimeouts())
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	return time.Since(start).Milliseconds(), nil
}

func (g *Gateway) evaluateEndToEndChainProfile(target profileEvaluationTarget, settings evaluationSettings) bool {
	startedAt := unixMillisNow()
	if !g.updateProfileEvaluationState(target, `state = 'running', last_error = '', last_evaluation_started_at = ?`, startedAt) {
		return false
	}
	exitNodes, err := g.loadExitNodes(target.ExitNodeIDs)
	if err != nil {
		if target.NodeStickyEnabled {
			if frontNodeID, exitNodeID := g.profileCurrentChainPath(target.ID); frontNodeID != "" && exitNodeID != "" && (g.profileRetainsNode(target.ID, frontNodeID) || g.profileRetainsNode(target.ID, exitNodeID)) {
				g.updateProfileEvaluationState(target, `state = 'degraded', last_error = ?, switch_reason = 'current_path_reused_after_failure', current_path_failed_evaluations = current_path_failed_evaluations + 1, current_path_missed_success_cycles = current_path_missed_success_cycles + 1, last_evaluated_at = ?`, err.Error(), unixMillisNow())
				return false
			}
		}
		g.updateProfileEvaluationState(target, `state = 'invalid_config', current_node_id = '', current_exit_node_id = '', last_error = ?, switch_reason = 'missing_exit_node', last_evaluated_at = ?`, err.Error(), unixMillisNow())
		return false
	}
	nodes, err := g.candidateNodes(target.Filter)
	if err != nil {
		g.updateProfileEvaluationState(target, `state = 'failed', current_node_id = '', current_exit_node_id = '', last_error = ?, switch_reason = 'candidate_filter_error', last_evaluated_at = ?`, err.Error(), unixMillisNow())
		return false
	}
	nodes = excludeNodes(nodes, target.ExitNodeIDs)
	if len(nodes) == 0 {
		if target.NodeStickyEnabled {
			if frontNodeID, exitNodeID := g.profileCurrentChainPath(target.ID); frontNodeID != "" && exitNodeID != "" && (g.profileRetainsNode(target.ID, frontNodeID) || g.profileRetainsNode(target.ID, exitNodeID)) {
				g.updateProfileEvaluationState(target, `state = 'degraded', last_error = 'no front node candidates', switch_reason = 'current_path_reused_after_failure', current_path_failed_evaluations = current_path_failed_evaluations + 1, current_path_missed_success_cycles = current_path_missed_success_cycles + 1, last_evaluated_at = ?`, unixMillisNow())
				return false
			}
		}
		g.updateProfileEvaluationState(target, `state = 'no_candidate', current_node_id = '', current_exit_node_id = '', last_error = 'no front node candidates', switch_reason = 'no_candidate', last_evaluated_at = ?`, unixMillisNow())
		return false
	}
	pairs := chainCandidatePairs(nodes, exitNodes)
	if len(pairs) == 0 {
		if target.NodeStickyEnabled {
			if frontNodeID, exitNodeID := g.profileCurrentChainPath(target.ID); frontNodeID != "" && exitNodeID != "" && (g.profileRetainsNode(target.ID, frontNodeID) || g.profileRetainsNode(target.ID, exitNodeID)) {
				g.updateProfileEvaluationState(target, `state = 'degraded', last_error = 'no chain path candidates', switch_reason = 'current_path_reused_after_failure', current_path_failed_evaluations = current_path_failed_evaluations + 1, current_path_missed_success_cycles = current_path_missed_success_cycles + 1, last_evaluated_at = ?`, unixMillisNow())
				return false
			}
		}
		g.updateProfileEvaluationState(target, `state = 'no_candidate', current_node_id = '', current_exit_node_id = '', last_error = 'no chain path candidates', switch_reason = 'no_candidate', last_evaluated_at = ?`, unixMillisNow())
		return false
	}

	currentNodeID, currentExitNodeID := g.profileCurrentChainPath(target.ID)
	if !chainPairInRecords(currentNodeID, currentExitNodeID, pairs) {
		if !(g.profileRetainsNode(target.ID, currentNodeID) || g.profileRetainsNode(target.ID, currentExitNodeID)) {
			currentNodeID = ""
			currentExitNodeID = ""
		}
	}
	results := g.evaluateChainCandidateProbes(
		pairs,
		settings.GlobalConcurrency,
		func(pair chainCandidatePair) chainCandidateProbeResult {
			duration, status, err := g.fetchTestURLThroughChain(pair.FrontNode, pair.ExitNode, target.TestURL, settings)
			return chainCandidateProbeResult{Pair: pair, Duration: duration, Status: status, Err: err}
		},
	)
	probeResults := make([]appevaluations.ChainProbeResult, 0, len(results))
	for _, result := range results {
		probeResults = append(probeResults, appevaluations.ChainProbeResult{
			FrontNodeID: result.Pair.FrontNode.ID,
			ExitNodeID:  result.Pair.ExitNode.ID,
			DurationMS:  result.Duration,
			OK:          result.succeeded(),
			Error:       result.failureMessage(),
		})
	}
	summary := appevaluations.SummarizeChainProbeResults(currentNodeID, currentExitNodeID, probeResults)

	finishedAt := unixMillisNow()
	if summary.BestFrontNodeID == "" || summary.BestExitNodeID == "" {
		if summary.LastFailure == "" {
			summary.LastFailure = "all chain path candidates failed"
		}
		if currentNodeID != "" && currentExitNodeID != "" {
			g.updateProfileEvaluationState(target, `state = 'degraded', last_error = ?, switch_reason = 'current_path_reused_after_failure', current_path_failed_evaluations = current_path_failed_evaluations + 1, current_path_missed_success_cycles = current_path_missed_success_cycles + 1, last_evaluated_at = ?`, summary.LastFailure, finishedAt)
			return false
		}
		g.updateProfileEvaluationState(target, `state = 'failed', current_node_id = '', current_exit_node_id = '', last_error = ?, switch_reason = 'all_candidates_failed', last_evaluated_at = ?`, summary.LastFailure, finishedAt)
		return false
	}
	selectedNodeID, selectedExitNodeID, selectedDuration, state, reason, failedCount, missedCycles := g.stableChainProfileSelection(target, currentNodeID, currentExitNodeID, summary.CurrentDurationMS, summary.BestFrontNodeID, summary.BestExitNodeID, summary.BestDurationMS)
	details := chainEvaluationDetails(target.TestURL, len(pairs), summary.FailureCount, summary.BestFrontNodeID, summary.BestExitNodeID, summary.BestDurationMS, currentNodeID, currentExitNodeID, summary.CurrentDurationMS, selectedNodeID, selectedExitNodeID, reason)
	if !g.selectedChainPathStillValid(target, selectedNodeID, selectedExitNodeID) {
		finishedAt = unixMillisNow()
		details["selected_node_id"] = selectedNodeID
		details["selected_exit_node_id"] = selectedExitNodeID
		details["switch_reason"] = "selected_node_removed"
		details["reason"] = "selected_node_removed"
		detailsJSON, _ := json.Marshal(details)
		g.updateProfileEvaluationState(
			target,
			`state = 'waiting_observation',
			 current_node_id = '',
			 current_exit_node_id = '',
			 last_error = 'selected chain path no longer exists',
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
		[]string{selectedNodeID, selectedExitNodeID},
		`state = ?,
		 current_node_id = ?,
		 current_exit_node_id = ?,
		 last_error = '',
		 current_path_latency_ms = ?,
		 current_path_failed_evaluations = ?,
		 current_path_missed_success_cycles = ?,
		 switch_reason = ?,
		 last_evaluation_details_json = ?,
		 last_evaluated_at = ?`,
		state,
		selectedNodeID,
		selectedExitNodeID,
		selectedDuration,
		failedCount,
		missedCycles,
		reason,
		string(detailsJSON),
		finishedAt,
	)
}

func (g *Gateway) fetchTestURLThroughChain(frontNode, exitNode nodeRecord, testURL string, settings evaluationSettings) (int64, int, error) {
	outbound, err := buildOutboundGETRequest(testURL)
	if err != nil {
		return 0, 0, err
	}
	start := time.Now()
	timeouts := settings.probeDialTimeouts()
	conn, err := g.dialViaChain(frontNode, exitNode, outbound.TargetHost, timeouts)
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

func (g *Gateway) loadExitNodes(exitNodeIDs []string) ([]nodeRecord, error) {
	exitNodeIDs = normalizeStringList(exitNodeIDs)
	if len(exitNodeIDs) == 0 {
		return nil, errors.New("exit_node_ids is required")
	}
	nodes := make([]nodeRecord, 0, len(exitNodeIDs))
	for _, exitNodeID := range exitNodeIDs {
		node, err := g.loadUsableNode(exitNodeID)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func chainCandidatePairs(frontNodes []nodeRecord, exitNodes []nodeRecord) []chainCandidatePair {
	pairs := make([]chainCandidatePair, 0, len(frontNodes)*len(exitNodes))
	for _, frontNode := range frontNodes {
		for _, exitNode := range exitNodes {
			pairs = append(pairs, chainCandidatePair{FrontNode: frontNode, ExitNode: exitNode})
		}
	}
	return pairs
}

func (g *Gateway) evaluateChainCandidateProbes(pairs []chainCandidatePair, concurrency int, probe func(chainCandidatePair) chainCandidateProbeResult) []chainCandidateProbeResult {
	if len(pairs) == 0 {
		return nil
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(pairs) {
		concurrency = len(pairs)
	}
	jobs := make(chan chainCandidatePair)
	results := make(chan chainCandidateProbeResult, len(pairs))
	done := make(chan struct{}, concurrency)
	for range concurrency {
		go func() {
			defer func() { done <- struct{}{} }()
			for pair := range jobs {
				results <- probe(pair)
			}
		}()
	}
	go func() {
		for _, pair := range pairs {
			jobs <- pair
		}
		close(jobs)
		for range concurrency {
			<-done
		}
		close(results)
	}()
	collected := make([]chainCandidateProbeResult, 0, len(pairs))
	for result := range results {
		collected = append(collected, result)
	}
	return collected
}

func excludeNodes(nodes []nodeRecord, nodeIDs []string) []nodeRecord {
	if len(nodeIDs) == 0 {
		return nodes
	}
	excluded := map[string]bool{}
	for _, nodeID := range nodeIDs {
		excluded[nodeID] = true
	}
	filtered := make([]nodeRecord, 0, len(nodes))
	for _, node := range nodes {
		if !excluded[node.ID] {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func (g *Gateway) profileCurrentChainPath(profileID string) (string, string) {
	var frontNodeID, exitNodeID string
	_ = g.db.QueryRow(`SELECT current_node_id, current_exit_node_id FROM access_profiles WHERE id = ?`, profileID).Scan(&frontNodeID, &exitNodeID)
	return frontNodeID, exitNodeID
}

func (g *Gateway) stableChainProfileSelection(target profileEvaluationTarget, currentNodeID, currentExitNodeID string, currentDuration int64, bestNodeID, bestExitNodeID string, bestDuration int64) (string, string, int64, string, string, int, int) {
	selection := appevaluations.SelectChainPath(appevaluations.ChainSelectionInput{
		CurrentFrontNodeID:           currentNodeID,
		CurrentExitNodeID:            currentExitNodeID,
		CurrentDurationMS:            currentDuration,
		BestFrontNodeID:              bestNodeID,
		BestExitNodeID:               bestExitNodeID,
		BestDurationMS:               bestDuration,
		RelativeImprovementThreshold: target.RelativeImprovementThreshold,
		AbsoluteLatencyImprovementMS: target.AbsoluteImprovementMS,
		ForceSwitch:                  target.ForceSwitch,
	})
	return selection.SelectedFrontNodeID, selection.SelectedExitNodeID, selection.SelectedDurationMS, selection.State, selection.SwitchReason, selection.FailedCount, selection.MissedSuccessCycles
}

func (g *Gateway) selectedChainPathStillValid(target profileEvaluationTarget, frontNodeID, exitNodeID string) bool {
	if !g.selectedProfileNodeStillValid(target, frontNodeID) {
		return false
	}
	if exitNodeID == "" || !stringInSlice(exitNodeID, target.ExitNodeIDs) {
		return false
	}
	exitNode, err := g.loadNode(exitNodeID)
	return err == nil && exitNode.Enabled
}

func chainPairInRecords(frontNodeID, exitNodeID string, pairs []chainCandidatePair) bool {
	if frontNodeID == "" || exitNodeID == "" {
		return false
	}
	for _, pair := range pairs {
		if pair.FrontNode.ID == frontNodeID && pair.ExitNode.ID == exitNodeID {
			return true
		}
	}
	return false
}

func chainEvaluationDetails(testURL string, candidates int, failures int, bestNodeID, bestExitNodeID string, bestDuration int64, currentNodeID, currentExitNodeID string, currentDuration int64, selectedNodeID, selectedExitNodeID string, reason string) map[string]any {
	return appevaluations.BuildChainDetails(appevaluations.ChainDetailsInput{
		TestURL:             testURL,
		CandidateCount:      candidates,
		FailureCount:        failures,
		BestFrontNodeID:     bestNodeID,
		BestExitNodeID:      bestExitNodeID,
		BestDurationMS:      bestDuration,
		CurrentFrontNodeID:  currentNodeID,
		CurrentExitNodeID:   currentExitNodeID,
		CurrentDurationMS:   currentDuration,
		SelectedFrontNodeID: selectedNodeID,
		SelectedExitNodeID:  selectedExitNodeID,
		SwitchReason:        reason,
	})
}

func clearlyBetter(candidateDuration, currentDuration int64, relativeThreshold float64, absoluteThresholdMS int) bool {
	if candidateDuration >= currentDuration {
		return false
	}
	improvement := currentDuration - candidateDuration
	if relativeThreshold <= 0 && absoluteThresholdMS <= 0 {
		return true
	}
	if relativeThreshold > 0 {
		relativeThresholdMS := int64(math.Ceil(float64(currentDuration) * relativeThreshold))
		if improvement >= relativeThresholdMS {
			return true
		}
	}
	if absoluteThresholdMS > 0 && improvement >= int64(absoluteThresholdMS) {
		return true
	}
	return false
}
