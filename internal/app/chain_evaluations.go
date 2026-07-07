package app

import (
	"context"

	appevaluations "proxygateway/internal/application/evaluations"
	probinginfra "proxygateway/internal/infrastructure/probing"

	"go.uber.org/zap"
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
	Timings  appevaluations.ProbeTimings
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
	client, closeClient, err := g.temporaryProbeClient()
	if err != nil {
		g.log().Warn("create temporary evaluation probe client failed", zap.String("profile_id", target.ID), zap.Error(err))
		return false
	}
	defer closeClient()
	return g.evaluationRunService(settings, client).EvaluateFastestFront(context.Background(), target, evaluationRuntimeSettings(settings))
}

func (g *Gateway) probeChainLink(frontNode, exitNode nodeRecord, settings evaluationSettings) (appevaluations.CandidateProbeMeasurement, error) {
	return probeChainLinkWithClient(g.probeClient(), frontNode, exitNode, settings)
}

func probeChainLinkWithClient(client probinginfra.Client, frontNode, exitNode nodeRecord, settings evaluationSettings) (appevaluations.CandidateProbeMeasurement, error) {
	result, err := client.ProbeChainLink(frontNode, exitNode, defaultChainLinkProbeURL, settings.probeDialTimeouts())
	return candidateProbeMeasurementFromDialResult(result), err
}

func (g *Gateway) evaluateEndToEndChainProfile(target profileEvaluationTarget, settings evaluationSettings) bool {
	client, closeClient, err := g.temporaryProbeClient()
	if err != nil {
		g.log().Warn("create temporary evaluation probe client failed", zap.String("profile_id", target.ID), zap.Error(err))
		return false
	}
	defer closeClient()
	return g.evaluationRunService(settings, client).EvaluateEndToEndChain(context.Background(), target, evaluationRuntimeSettings(settings))
}

func (g *Gateway) fetchTestURLThroughChain(frontNode, exitNode nodeRecord, testURL string, settings evaluationSettings) (appevaluations.CandidateProbeMeasurement, error) {
	return fetchTestURLThroughChainWithClient(g.probeClient(), frontNode, exitNode, testURL, settings)
}

func fetchTestURLThroughChainWithClient(client probinginfra.Client, frontNode, exitNode nodeRecord, testURL string, settings evaluationSettings) (appevaluations.CandidateProbeMeasurement, error) {
	result, err := client.FetchThroughChain(frontNode, exitNode, testURL, settings.probeDialTimeouts())
	return candidateProbeMeasurementFromHTTPResult(result), err
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
	return g.profileCurrentChainPathWithContext(context.Background(), profileID)
}

func (g *Gateway) profileCurrentChainPathWithContext(ctx context.Context, profileID string) (string, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	path, err := g.evaluationRepo.CurrentChainPath(ctx, profileID)
	if err != nil {
		return "", ""
	}
	return path.FrontNodeID, path.ExitNodeID
}

func (g *Gateway) selectedChainPathStillValid(target profileEvaluationTarget, frontNodeID, exitNodeID string) bool {
	return g.selectedChainPathStillValidWithContext(context.Background(), target, frontNodeID, exitNodeID)
}

func (g *Gateway) selectedChainPathStillValidWithContext(ctx context.Context, target profileEvaluationTarget, frontNodeID, exitNodeID string) bool {
	if !g.selectedProfileNodeStillValidWithContext(ctx, target, frontNodeID) {
		return false
	}
	if exitNodeID == "" || !stringInSlice(exitNodeID, target.ExitNodeIDs) {
		return false
	}
	exitNode, err := g.loadNodeWithContext(ctx, exitNodeID)
	return err == nil && exitNode.Enabled
}

func (g *Gateway) retainedChainLinkExitPathExists(target profileEvaluationTarget, exitNodeID string) bool {
	return g.retainedChainLinkExitPathExistsWithContext(context.Background(), target, exitNodeID)
}

func (g *Gateway) retainedChainLinkExitPathExistsWithContext(ctx context.Context, target profileEvaluationTarget, exitNodeID string) bool {
	if !target.NodeStickyEnabled {
		return false
	}
	frontNodeID, currentExitNodeID := g.profileCurrentChainPathWithContext(ctx, target.ID)
	return frontNodeID != "" && currentExitNodeID == exitNodeID && g.profileRetainsNodeWithContext(ctx, target.ID, currentExitNodeID)
}

func (g *Gateway) retainedCurrentChainPathExists(target profileEvaluationTarget) bool {
	return g.retainedCurrentChainPathExistsWithContext(context.Background(), target)
}

func (g *Gateway) retainedCurrentChainPathExistsWithContext(ctx context.Context, target profileEvaluationTarget) bool {
	if !target.NodeStickyEnabled {
		return false
	}
	frontNodeID, exitNodeID := g.profileCurrentChainPathWithContext(ctx, target.ID)
	return frontNodeID != "" && exitNodeID != "" && (g.profileRetainsNodeWithContext(ctx, target.ID, frontNodeID) || g.profileRetainsNodeWithContext(ctx, target.ID, exitNodeID))
}
