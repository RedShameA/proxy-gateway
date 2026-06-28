package evaluations

import (
	"context"
	"encoding/json"
	"errors"

	appproxy "proxygateway/internal/application/proxy"
	domainprofile "proxygateway/internal/domain/profile"
)

var errExitNodeIDsRequired = errors.New("exit_node_ids is required")

type RuntimeSettings struct {
	GlobalConcurrency    int
	SingleCandidateLimit int
	ChainCandidateLimit  int
}

type ChainCandidatePair struct {
	FrontNode appproxy.Node
	ExitNode  appproxy.Node
}

type ChainCandidateProbeLog struct {
	Pair       ChainCandidatePair
	DurationMS int64
	HTTPStatus int
	Timings    ProbeTimings
	Err        error
}

type Clock interface {
	NowMillis() int64
}

type CandidatePort interface {
	CandidateNodes(context.Context, domainprofile.CandidateFilter) ([]appproxy.Node, error)
	LoadUsableNode(context.Context, string) (appproxy.Node, error)
}

type ProfilePathPort interface {
	ProfileRetainsNode(context.Context, string, string) bool
	ProfileCurrentNodeID(context.Context, string) string
	ProfileCurrentChainPath(context.Context, string) (string, string)
	SelectedProfileNodeStillValid(context.Context, Target, string) bool
	SelectedChainPathStillValid(context.Context, Target, string, string) bool
	RetainedCurrentChainPathExists(context.Context, Target) bool
	RetainedChainLinkExitPathExists(context.Context, Target, string) bool
}

type StatePort interface {
	UpdateState(context.Context, Target, StateUpdate) bool
	UpdateStateAndReleaseRetained(context.Context, Target, []string, StateUpdate) bool
}

type ProbePort interface {
	FetchNode(context.Context, appproxy.Node, string) (CandidateProbeMeasurement, error)
	ProbeChainLink(context.Context, appproxy.Node, appproxy.Node) (CandidateProbeMeasurement, error)
	FetchChain(context.Context, appproxy.Node, appproxy.Node, string) (CandidateProbeMeasurement, error)
}

type RunObserver interface {
	LogFastestCandidate(context.Context, string, CandidateProbeResult[appproxy.Node])
	LogChainCandidate(context.Context, string, ChainCandidateProbeLog)
	LogFastestSelection(context.Context, Target, int, FastestProbeSummary, string, string, string)
	LogChainSelection(context.Context, Target, int, ChainProbeSummary, string, string, string, string, string)
}

type RunServiceDeps struct {
	Clock      Clock
	Candidates CandidatePort
	Paths      ProfilePathPort
	State      StatePort
	Probes     ProbePort
	Observer   RunObserver
}

type RunService struct {
	deps RunServiceDeps
}

func NewRunService(deps RunServiceDeps) RunService {
	if deps.Observer == nil {
		deps.Observer = noopRunObserver{}
	}
	return RunService{deps: deps}
}

func (s RunService) EvaluateFastest(ctx context.Context, target Target, settings RuntimeSettings) bool {
	ctx = runContext(ctx)
	startedAt := s.nowMillis()
	if !s.updateState(ctx, target, RunningStateUpdate(startedAt)) {
		return false
	}
	nodes, err := s.candidateNodes(ctx, target.Filter)
	if err != nil || len(nodes) == 0 {
		lastError := "no candidate nodes"
		if err != nil {
			lastError = err.Error()
		}
		currentNodeID := s.profileCurrentNodeID(ctx, target.ID)
		outcome := PlanFastestNoCandidate(lastError, target.NodeStickyEnabled && s.profileRetainsNode(ctx, target.ID, currentNodeID))
		s.updateState(ctx, target, outcome.StateUpdate(s.nowMillis()))
		return false
	}
	nodes = limitNodes(nodes, effectiveCandidateLimit(target.CandidateLimit, settings.SingleCandidateLimit))
	currentNodeID := s.profileCurrentNodeID(ctx, target.ID)
	if currentNodeID != "" && !nodeIDInRecords(currentNodeID, nodes) {
		if !s.profileRetainsNode(ctx, target.ID, currentNodeID) {
			currentNodeID = ""
		}
	}
	probeRun := ExecuteFastestCandidateProbes(
		nodes,
		settings.GlobalConcurrency,
		currentNodeID,
		func(node appproxy.Node) string { return node.ID },
		func(node appproxy.Node) (CandidateProbeMeasurement, error) {
			return s.fetchNode(ctx, node, target.TestURL)
		},
	)
	for _, result := range probeRun.Results {
		s.logFastestCandidate(ctx, target.ID, result)
	}
	summary := probeRun.Summary
	finishedAt := s.nowMillis()
	if summary.BestNodeID == "" {
		outcome := PlanFastestAllCandidatesFailed(currentNodeID, summary.LastFailure)
		s.updateState(ctx, target, outcome.StateUpdate(finishedAt))
		return false
	}
	selection := SelectFastestPath(FastestSelectionInput{
		CurrentNodeID:                currentNodeID,
		CurrentDurationMS:            summary.CurrentDurationMS,
		BestNodeID:                   summary.BestNodeID,
		BestDurationMS:               summary.BestDurationMS,
		RelativeImprovementThreshold: target.RelativeImprovementThreshold,
		AbsoluteLatencyImprovementMS: target.AbsoluteImprovementMS,
		ForceSwitch:                  target.ForceSwitch,
	})
	details := BuildFastestDetails(FastestDetailsInput{
		TestURL:           target.TestURL,
		CandidateCount:    len(nodes),
		FailureCount:      summary.FailureCount,
		BestNodeID:        summary.BestNodeID,
		BestDurationMS:    summary.BestDurationMS,
		CurrentNodeID:     currentNodeID,
		CurrentDurationMS: summary.CurrentDurationMS,
		SelectedNodeID:    selection.SelectedNodeID,
		SwitchReason:      selection.SwitchReason,
	})
	if !s.selectedProfileNodeStillValid(ctx, target, selection.SelectedNodeID) {
		finishedAt = s.nowMillis()
		details["selected_node_id"] = selection.SelectedNodeID
		details["switch_reason"] = SwitchReasonSelectedNodeRemoved
		details["reason"] = SwitchReasonSelectedNodeRemoved
		s.logFastestSelection(ctx, target, len(nodes), summary, currentNodeID, selection.SelectedNodeID, SwitchReasonSelectedNodeRemoved)
		detailsJSON, _ := json.Marshal(details)
		s.updateState(ctx, target, FastestSelectedNodeRemovedUpdate(string(detailsJSON), finishedAt))
		return false
	}
	s.logFastestSelection(ctx, target, len(nodes), summary, currentNodeID, selection.SelectedNodeID, selection.SwitchReason)
	detailsJSON, _ := json.Marshal(details)
	return s.updateStateAndReleaseRetained(
		ctx,
		target,
		[]string{selection.SelectedNodeID},
		selection.StateUpdate(string(detailsJSON), finishedAt),
	)
}

func (s RunService) EvaluateFastestFront(ctx context.Context, target Target, settings RuntimeSettings) bool {
	ctx = runContext(ctx)
	startedAt := s.nowMillis()
	if !s.updateState(ctx, target, RunningStateUpdate(startedAt)) {
		return false
	}
	if len(target.ExitNodeIDs) != 1 {
		outcome := PlanChainInvalidConfig("chain_link requires exactly one exit_node_id", SwitchReasonInvalidChainConfig)
		s.updateState(ctx, target, outcome.StateUpdate(s.nowMillis()))
		return false
	}
	exitNode, err := s.loadUsableNode(ctx, target.ExitNodeIDs[0])
	if err != nil {
		retainCurrentPath := s.retainedChainLinkExitPathExists(ctx, target, target.ExitNodeIDs[0])
		outcome := PlanChainMissingExitNode(err.Error(), retainCurrentPath)
		s.updateState(ctx, target, outcome.StateUpdate(s.nowMillis()))
		return false
	}
	nodes, err := s.candidateNodes(ctx, target.Filter)
	if err != nil {
		outcome := PlanChainCandidateFilterError(err.Error())
		s.updateState(ctx, target, outcome.StateUpdate(s.nowMillis()))
		return false
	}
	nodes = excludeNodes(nodes, target.ExitNodeIDs)
	if len(nodes) == 0 {
		outcome := PlanChainNoFrontCandidate(s.retainedCurrentChainPathExists(ctx, target))
		s.updateState(ctx, target, outcome.StateUpdate(s.nowMillis()))
		return false
	}
	currentNodeID, currentExitNodeID := s.profileCurrentChainPath(ctx, target.ID)
	if currentExitNodeID == "" {
		currentExitNodeID = exitNode.ID
	}
	if currentExitNodeID != exitNode.ID || !nodeIDInRecords(currentNodeID, nodes) {
		if !(s.profileRetainsNode(ctx, target.ID, currentNodeID) || s.profileRetainsNode(ctx, target.ID, currentExitNodeID)) {
			currentNodeID = ""
			currentExitNodeID = ""
		}
	}
	probeRun := ExecuteChainCandidateProbes(
		nodes,
		settings.GlobalConcurrency,
		currentNodeID,
		currentExitNodeID,
		func(node appproxy.Node) string { return node.ID },
		func(appproxy.Node) string { return exitNode.ID },
		func(node appproxy.Node) (CandidateProbeMeasurement, error) {
			return s.probeChainLink(ctx, node, exitNode)
		},
	)
	for _, result := range probeRun.Results {
		s.logChainCandidate(ctx, target.ID, ChainCandidateProbeLog{
			Pair:       ChainCandidatePair{FrontNode: result.Candidate, ExitNode: exitNode},
			DurationMS: result.DurationMS,
			HTTPStatus: result.HTTPStatus,
			Timings:    result.Timings,
			Err:        result.Err,
		})
	}
	summary := probeRun.Summary
	finishedAt := s.nowMillis()
	if summary.BestFrontNodeID == "" {
		outcome := PlanChainAllCandidatesFailed(currentNodeID != "" && currentExitNodeID != "", summary.LastFailure, "all front node candidates failed")
		s.updateState(ctx, target, outcome.StateUpdate(finishedAt))
		return false
	}
	return s.finishChainSelection(ctx, target, "chain-link", len(nodes), summary, currentNodeID, currentExitNodeID, finishedAt)
}

func (s RunService) EvaluateEndToEndChain(ctx context.Context, target Target, settings RuntimeSettings) bool {
	ctx = runContext(ctx)
	startedAt := s.nowMillis()
	if !s.updateState(ctx, target, RunningStateUpdate(startedAt)) {
		return false
	}
	exitNodes, err := s.loadExitNodes(ctx, target.ExitNodeIDs)
	if err != nil {
		outcome := PlanChainMissingExitNode(err.Error(), s.retainedCurrentChainPathExists(ctx, target))
		s.updateState(ctx, target, outcome.StateUpdate(s.nowMillis()))
		return false
	}
	nodes, err := s.candidateNodes(ctx, target.Filter)
	if err != nil {
		outcome := PlanChainCandidateFilterError(err.Error())
		s.updateState(ctx, target, outcome.StateUpdate(s.nowMillis()))
		return false
	}
	nodes = excludeNodes(nodes, target.ExitNodeIDs)
	if len(nodes) == 0 {
		outcome := PlanChainNoFrontCandidate(s.retainedCurrentChainPathExists(ctx, target))
		s.updateState(ctx, target, outcome.StateUpdate(s.nowMillis()))
		return false
	}
	pairs := chainCandidatePairs(nodes, exitNodes)
	if len(pairs) == 0 {
		outcome := PlanChainNoPathCandidate(s.retainedCurrentChainPathExists(ctx, target))
		s.updateState(ctx, target, outcome.StateUpdate(s.nowMillis()))
		return false
	}
	currentNodeID, currentExitNodeID := s.profileCurrentChainPath(ctx, target.ID)
	if !chainPairInRecords(currentNodeID, currentExitNodeID, pairs) {
		if !(s.profileRetainsNode(ctx, target.ID, currentNodeID) || s.profileRetainsNode(ctx, target.ID, currentExitNodeID)) {
			currentNodeID = ""
			currentExitNodeID = ""
		}
	}
	probeRun := ExecuteChainCandidateProbes(
		pairs,
		settings.GlobalConcurrency,
		currentNodeID,
		currentExitNodeID,
		func(pair ChainCandidatePair) string { return pair.FrontNode.ID },
		func(pair ChainCandidatePair) string { return pair.ExitNode.ID },
		func(pair ChainCandidatePair) (CandidateProbeMeasurement, error) {
			return s.fetchChain(ctx, pair.FrontNode, pair.ExitNode, target.TestURL)
		},
	)
	for _, result := range probeRun.Results {
		s.logChainCandidate(ctx, target.ID, ChainCandidateProbeLog{
			Pair:       result.Candidate,
			DurationMS: result.DurationMS,
			HTTPStatus: result.HTTPStatus,
			Timings:    result.Timings,
			Err:        result.Err,
		})
	}
	summary := probeRun.Summary
	finishedAt := s.nowMillis()
	if summary.BestFrontNodeID == "" || summary.BestExitNodeID == "" {
		outcome := PlanChainAllCandidatesFailed(currentNodeID != "" && currentExitNodeID != "", summary.LastFailure, "all chain path candidates failed")
		s.updateState(ctx, target, outcome.StateUpdate(finishedAt))
		return false
	}
	return s.finishChainSelection(ctx, target, target.TestURL, len(pairs), summary, currentNodeID, currentExitNodeID, finishedAt)
}

func (s RunService) finishChainSelection(ctx context.Context, target Target, testURL string, candidateCount int, summary ChainProbeSummary, currentNodeID, currentExitNodeID string, finishedAt int64) bool {
	selection := SelectChainPath(ChainSelectionInput{
		CurrentFrontNodeID:           currentNodeID,
		CurrentExitNodeID:            currentExitNodeID,
		CurrentDurationMS:            summary.CurrentDurationMS,
		BestFrontNodeID:              summary.BestFrontNodeID,
		BestExitNodeID:               summary.BestExitNodeID,
		BestDurationMS:               summary.BestDurationMS,
		RelativeImprovementThreshold: target.RelativeImprovementThreshold,
		AbsoluteLatencyImprovementMS: target.AbsoluteImprovementMS,
		ForceSwitch:                  target.ForceSwitch,
	})
	details := BuildChainDetails(ChainDetailsInput{
		TestURL:             testURL,
		CandidateCount:      candidateCount,
		FailureCount:        summary.FailureCount,
		BestFrontNodeID:     summary.BestFrontNodeID,
		BestExitNodeID:      summary.BestExitNodeID,
		BestDurationMS:      summary.BestDurationMS,
		CurrentFrontNodeID:  currentNodeID,
		CurrentExitNodeID:   currentExitNodeID,
		CurrentDurationMS:   summary.CurrentDurationMS,
		SelectedFrontNodeID: selection.SelectedFrontNodeID,
		SelectedExitNodeID:  selection.SelectedExitNodeID,
		SwitchReason:        selection.SwitchReason,
	})
	if !s.selectedChainPathStillValid(ctx, target, selection.SelectedFrontNodeID, selection.SelectedExitNodeID) {
		finishedAt = s.nowMillis()
		details["selected_node_id"] = selection.SelectedFrontNodeID
		details["selected_exit_node_id"] = selection.SelectedExitNodeID
		details["switch_reason"] = SwitchReasonSelectedNodeRemoved
		details["reason"] = SwitchReasonSelectedNodeRemoved
		s.logChainSelection(ctx, target, candidateCount, summary, currentNodeID, currentExitNodeID, selection.SelectedFrontNodeID, selection.SelectedExitNodeID, SwitchReasonSelectedNodeRemoved)
		detailsJSON, _ := json.Marshal(details)
		s.updateState(ctx, target, ChainSelectedPathRemovedUpdate(string(detailsJSON), finishedAt))
		return false
	}
	s.logChainSelection(ctx, target, candidateCount, summary, currentNodeID, currentExitNodeID, selection.SelectedFrontNodeID, selection.SelectedExitNodeID, selection.SwitchReason)
	detailsJSON, _ := json.Marshal(details)
	return s.updateStateAndReleaseRetained(
		ctx,
		target,
		[]string{selection.SelectedFrontNodeID, selection.SelectedExitNodeID},
		selection.StateUpdate(string(detailsJSON), finishedAt),
	)
}

func (s RunService) loadExitNodes(ctx context.Context, exitNodeIDs []string) ([]appproxy.Node, error) {
	exitNodeIDs = normalizeStringList(exitNodeIDs)
	if len(exitNodeIDs) == 0 {
		return nil, errExitNodeIDsRequired
	}
	nodes := make([]appproxy.Node, 0, len(exitNodeIDs))
	for _, exitNodeID := range exitNodeIDs {
		node, err := s.loadUsableNode(ctx, exitNodeID)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (s RunService) nowMillis() int64 {
	if s.deps.Clock == nil {
		return 0
	}
	return s.deps.Clock.NowMillis()
}

func (s RunService) candidateNodes(ctx context.Context, filter domainprofile.CandidateFilter) ([]appproxy.Node, error) {
	if s.deps.Candidates == nil {
		return nil, nil
	}
	return s.deps.Candidates.CandidateNodes(ctx, filter)
}

func (s RunService) loadUsableNode(ctx context.Context, nodeID string) (appproxy.Node, error) {
	if s.deps.Candidates == nil {
		return appproxy.Node{}, errors.New("candidate port is not configured")
	}
	return s.deps.Candidates.LoadUsableNode(ctx, nodeID)
}

func (s RunService) profileRetainsNode(ctx context.Context, profileID, nodeID string) bool {
	return s.deps.Paths != nil && s.deps.Paths.ProfileRetainsNode(ctx, profileID, nodeID)
}

func (s RunService) profileCurrentNodeID(ctx context.Context, profileID string) string {
	if s.deps.Paths == nil {
		return ""
	}
	return s.deps.Paths.ProfileCurrentNodeID(ctx, profileID)
}

func (s RunService) profileCurrentChainPath(ctx context.Context, profileID string) (string, string) {
	if s.deps.Paths == nil {
		return "", ""
	}
	return s.deps.Paths.ProfileCurrentChainPath(ctx, profileID)
}

func (s RunService) updateState(ctx context.Context, target Target, update StateUpdate) bool {
	return s.deps.State != nil && s.deps.State.UpdateState(ctx, target, update)
}

func (s RunService) updateStateAndReleaseRetained(ctx context.Context, target Target, keepNodeIDs []string, update StateUpdate) bool {
	return s.deps.State != nil && s.deps.State.UpdateStateAndReleaseRetained(ctx, target, keepNodeIDs, update)
}

func (s RunService) selectedProfileNodeStillValid(ctx context.Context, target Target, nodeID string) bool {
	return s.deps.Paths == nil || s.deps.Paths.SelectedProfileNodeStillValid(ctx, target, nodeID)
}

func (s RunService) selectedChainPathStillValid(ctx context.Context, target Target, frontNodeID, exitNodeID string) bool {
	return s.deps.Paths == nil || s.deps.Paths.SelectedChainPathStillValid(ctx, target, frontNodeID, exitNodeID)
}

func (s RunService) retainedCurrentChainPathExists(ctx context.Context, target Target) bool {
	return s.deps.Paths != nil && s.deps.Paths.RetainedCurrentChainPathExists(ctx, target)
}

func (s RunService) retainedChainLinkExitPathExists(ctx context.Context, target Target, exitNodeID string) bool {
	return s.deps.Paths != nil && s.deps.Paths.RetainedChainLinkExitPathExists(ctx, target, exitNodeID)
}

func (s RunService) fetchNode(ctx context.Context, node appproxy.Node, testURL string) (CandidateProbeMeasurement, error) {
	if s.deps.Probes == nil {
		return CandidateProbeMeasurement{}, errors.New("probe port is not configured")
	}
	return s.deps.Probes.FetchNode(ctx, node, testURL)
}

func (s RunService) probeChainLink(ctx context.Context, frontNode, exitNode appproxy.Node) (CandidateProbeMeasurement, error) {
	if s.deps.Probes == nil {
		return CandidateProbeMeasurement{}, errors.New("probe port is not configured")
	}
	return s.deps.Probes.ProbeChainLink(ctx, frontNode, exitNode)
}

func (s RunService) fetchChain(ctx context.Context, frontNode, exitNode appproxy.Node, testURL string) (CandidateProbeMeasurement, error) {
	if s.deps.Probes == nil {
		return CandidateProbeMeasurement{}, errors.New("probe port is not configured")
	}
	return s.deps.Probes.FetchChain(ctx, frontNode, exitNode, testURL)
}

func (s RunService) logFastestCandidate(ctx context.Context, profileID string, result CandidateProbeResult[appproxy.Node]) {
	if s.deps.Observer != nil {
		s.deps.Observer.LogFastestCandidate(ctx, profileID, result)
	}
}

func (s RunService) logChainCandidate(ctx context.Context, profileID string, result ChainCandidateProbeLog) {
	if s.deps.Observer != nil {
		s.deps.Observer.LogChainCandidate(ctx, profileID, result)
	}
}

func (s RunService) logFastestSelection(ctx context.Context, target Target, candidateCount int, summary FastestProbeSummary, currentNodeID, selectedNodeID, switchReason string) {
	if s.deps.Observer != nil {
		s.deps.Observer.LogFastestSelection(ctx, target, candidateCount, summary, currentNodeID, selectedNodeID, switchReason)
	}
}

func (s RunService) logChainSelection(ctx context.Context, target Target, candidateCount int, summary ChainProbeSummary, currentNodeID, currentExitNodeID, selectedNodeID, selectedExitNodeID, switchReason string) {
	if s.deps.Observer != nil {
		s.deps.Observer.LogChainSelection(ctx, target, candidateCount, summary, currentNodeID, currentExitNodeID, selectedNodeID, selectedExitNodeID, switchReason)
	}
}

type noopRunObserver struct{}

func (noopRunObserver) LogFastestCandidate(context.Context, string, CandidateProbeResult[appproxy.Node]) {
}

func (noopRunObserver) LogChainCandidate(context.Context, string, ChainCandidateProbeLog) {
}

func (noopRunObserver) LogFastestSelection(context.Context, Target, int, FastestProbeSummary, string, string, string) {
}

func (noopRunObserver) LogChainSelection(context.Context, Target, int, ChainProbeSummary, string, string, string, string, string) {
}

func runContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func limitNodes(nodes []appproxy.Node, limit int) []appproxy.Node {
	if limit <= 0 || len(nodes) <= limit {
		return nodes
	}
	return nodes[:limit]
}

func nodeIDInRecords(nodeID string, nodes []appproxy.Node) bool {
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

func chainCandidatePairs(frontNodes []appproxy.Node, exitNodes []appproxy.Node) []ChainCandidatePair {
	pairs := make([]ChainCandidatePair, 0, len(frontNodes)*len(exitNodes))
	for _, frontNode := range frontNodes {
		for _, exitNode := range exitNodes {
			if frontNode.ID == exitNode.ID {
				continue
			}
			pairs = append(pairs, ChainCandidatePair{FrontNode: frontNode, ExitNode: exitNode})
		}
	}
	return pairs
}

func chainPairInRecords(frontNodeID, exitNodeID string, pairs []ChainCandidatePair) bool {
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

func excludeNodes(nodes []appproxy.Node, nodeIDs []string) []appproxy.Node {
	if len(nodes) == 0 || len(nodeIDs) == 0 {
		return nodes
	}
	excluded := map[string]bool{}
	for _, id := range nodeIDs {
		excluded[id] = true
	}
	filtered := make([]appproxy.Node, 0, len(nodes))
	for _, node := range nodes {
		if !excluded[node.ID] {
			filtered = append(filtered, node)
		}
	}
	return filtered
}
