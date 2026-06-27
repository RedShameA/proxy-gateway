package app

import (
	"context"

	appevaluations "proxygateway/internal/application/evaluations"
)

func (g *Gateway) evaluationRunService(settings evaluationSettings) appevaluations.RunService {
	return appevaluations.NewRunService(appevaluations.RunServiceDeps{
		Clock:      evaluationClock{},
		Candidates: evaluationCandidatePort{g: g},
		Paths:      evaluationPathPort{g: g},
		State:      evaluationStatePort{g: g},
		Probes:     evaluationProbePort{g: g, settings: settings},
		Observer:   evaluationRunObserver{g: g},
	})
}

type evaluationClock struct{}

func (evaluationClock) NowMillis() int64 {
	return unixMillisNow()
}

type evaluationCandidatePort struct {
	g *Gateway
}

func (p evaluationCandidatePort) CandidateNodes(ctx context.Context, filter candidateFilter) ([]nodeRecord, error) {
	return p.g.candidateNodesWithContext(ctx, filter)
}

func (p evaluationCandidatePort) LoadUsableNode(ctx context.Context, nodeID string) (nodeRecord, error) {
	return p.g.loadUsableNodeWithContext(ctx, nodeID)
}

type evaluationPathPort struct {
	g *Gateway
}

func (p evaluationPathPort) ProfileRetainsNode(ctx context.Context, profileID, nodeID string) bool {
	return p.g.profileRetainsNodeWithContext(ctx, profileID, nodeID)
}

func (p evaluationPathPort) ProfileCurrentNodeID(ctx context.Context, profileID string) string {
	return p.g.profileCurrentNodeIDWithContext(ctx, profileID)
}

func (p evaluationPathPort) ProfileCurrentChainPath(ctx context.Context, profileID string) (string, string) {
	return p.g.profileCurrentChainPathWithContext(ctx, profileID)
}

func (p evaluationPathPort) SelectedProfileNodeStillValid(ctx context.Context, target profileEvaluationTarget, nodeID string) bool {
	return p.g.selectedProfileNodeStillValidWithContext(ctx, target, nodeID)
}

func (p evaluationPathPort) SelectedChainPathStillValid(ctx context.Context, target profileEvaluationTarget, frontNodeID, exitNodeID string) bool {
	return p.g.selectedChainPathStillValidWithContext(ctx, target, frontNodeID, exitNodeID)
}

func (p evaluationPathPort) RetainedCurrentChainPathExists(ctx context.Context, target profileEvaluationTarget) bool {
	return p.g.retainedCurrentChainPathExistsWithContext(ctx, target)
}

func (p evaluationPathPort) RetainedChainLinkExitPathExists(ctx context.Context, target profileEvaluationTarget, exitNodeID string) bool {
	return p.g.retainedChainLinkExitPathExistsWithContext(ctx, target, exitNodeID)
}

type evaluationStatePort struct {
	g *Gateway
}

func (p evaluationStatePort) UpdateState(ctx context.Context, target profileEvaluationTarget, update appevaluations.StateUpdate) bool {
	return p.g.updateProfileEvaluationStateWithContext(ctx, target, update)
}

func (p evaluationStatePort) UpdateStateAndReleaseRetained(ctx context.Context, target profileEvaluationTarget, keepNodeIDs []string, update appevaluations.StateUpdate) bool {
	return p.g.updateProfileEvaluationStateAndReleaseRetainedWithContext(ctx, target, keepNodeIDs, update)
}

type evaluationProbePort struct {
	g        *Gateway
	settings evaluationSettings
}

func (p evaluationProbePort) FetchNode(_ context.Context, node nodeRecord, testURL string) (int64, int, error) {
	return p.g.fetchTestURLThroughNode(node, testURL, p.settings)
}

func (p evaluationProbePort) ProbeChainLink(_ context.Context, frontNode, exitNode nodeRecord) (int64, error) {
	return p.g.probeChainLink(frontNode, exitNode, p.settings)
}

func (p evaluationProbePort) FetchChain(_ context.Context, frontNode, exitNode nodeRecord, testURL string) (int64, int, error) {
	return p.g.fetchTestURLThroughChain(frontNode, exitNode, testURL, p.settings)
}

type evaluationRunObserver struct {
	g *Gateway
}

func (o evaluationRunObserver) LogFastestCandidate(_ context.Context, profileID string, result appevaluations.CandidateProbeResult[nodeRecord]) {
	o.g.logFastestCandidateProbe(profileID, profileCandidateProbeResult{
		Node:     result.Candidate,
		Duration: result.DurationMS,
		Status:   result.HTTPStatus,
		Err:      result.Err,
	})
}

func (o evaluationRunObserver) LogChainCandidate(_ context.Context, profileID string, result appevaluations.ChainCandidateProbeLog) {
	o.g.logChainCandidateProbe(profileID, chainCandidateProbeResult{
		Pair: chainCandidatePair{
			FrontNode: result.Pair.FrontNode,
			ExitNode:  result.Pair.ExitNode,
		},
		Duration: result.DurationMS,
		Status:   result.HTTPStatus,
		Err:      result.Err,
	})
}

func (o evaluationRunObserver) LogFastestSelection(_ context.Context, target profileEvaluationTarget, candidateCount int, summary appevaluations.FastestProbeSummary, currentNodeID, selectedNodeID, switchReason string) {
	o.g.logFastestEvaluationSelection(target, candidateCount, summary, currentNodeID, selectedNodeID, switchReason)
}

func (o evaluationRunObserver) LogChainSelection(_ context.Context, target profileEvaluationTarget, candidateCount int, summary appevaluations.ChainProbeSummary, currentNodeID, currentExitNodeID, selectedNodeID, selectedExitNodeID, switchReason string) {
	o.g.logChainEvaluationSelection(target, candidateCount, summary, currentNodeID, currentExitNodeID, selectedNodeID, selectedExitNodeID, switchReason)
}
