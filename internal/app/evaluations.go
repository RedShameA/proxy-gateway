package app

import (
	"context"
	"errors"

	appevaluations "proxygateway/internal/application/evaluations"
	domainprofile "proxygateway/internal/domain/profile"
	probinginfra "proxygateway/internal/infrastructure/probing"
)

const defaultProfileTestURL = "https://www.gstatic.com/generate_204"

func (g *Gateway) runManualProfileEvaluations(forceSwitch bool) (int, int, error) {
	settings, err := g.loadEvaluationSettings()
	if err != nil {
		return 0, 0, errors.New("load evaluation settings")
	}
	maintenanceSettings, err := g.loadMaintenanceSettings()
	if err != nil {
		return 0, 0, errors.New("load maintenance settings")
	}
	records, err := g.evaluationRepo.ListTargets(context.Background())
	if err != nil {
		return 0, 0, errors.New("list profiles")
	}
	var targets []profileEvaluationTarget
	now := unixMillisNow()
	skipped := 0
	for _, record := range records {
		target, shouldSkip := g.profileEvaluationTargetFromRecord(record, settings, forceSwitch, now)
		if shouldSkip {
			skipped++
			continue
		}
		targets = append(targets, target)
	}
	evaluated := g.runProfileEvaluations(targets, settings, maintenanceSettings.ProfileEvaluationConcurrency)
	return evaluated, skipped, nil
}

func (g *Gateway) runProfileEvaluations(targets []profileEvaluationTarget, settings evaluationSettings, profileConcurrency int) int {
	return len(appevaluations.RunConcurrentProbes(targets, profileConcurrency, func(target profileEvaluationTarget) bool {
		return g.runOneProfileEvaluation(target, settings)
	}))
}

func (g *Gateway) profileEvaluationTarget(profileID string, forceSwitch bool) (profileEvaluationTarget, evaluationSettings, bool, error) {
	settings, err := g.loadEvaluationSettings()
	if err != nil {
		return profileEvaluationTarget{}, evaluationSettings{}, false, err
	}
	record, found, err := g.evaluationRepo.LoadTarget(context.Background(), profileID)
	if err != nil {
		return profileEvaluationTarget{}, settings, false, err
	}
	if !found || !profileTypeNeedsEvaluation(record.Type) {
		return profileEvaluationTarget{}, settings, true, nil
	}
	target, skipped := g.profileEvaluationTargetFromRecord(record, settings, forceSwitch, unixMillisNow())
	return target, settings, skipped, nil
}

func (g *Gateway) profileEvaluationTargetFromRecord(record appevaluations.TargetRecord, settings evaluationSettings, forceSwitch bool, now int64) (profileEvaluationTarget, bool) {
	return appevaluations.BuildTarget(appevaluations.TargetBuildInput{
		Record:                              record,
		DefaultTestURL:                      defaultProfileTestURL,
		DefaultMinEvaluationIntervalSeconds: settings.DefaultMinEvaluationIntervalSeconds,
		NowMS:                               now,
		ForceSwitch:                         forceSwitch,
	})
}

func (g *Gateway) runOneProfileEvaluation(target profileEvaluationTarget, settings evaluationSettings) bool {
	switch target.Type {
	case domainprofile.TypeFastest:
		return g.evaluateFastestProfile(target, settings)
	case domainprofile.TypeChain:
		if normalizeChainEvaluationMode(target.ChainEvaluationMode) == domainprofile.ChainEvaluationModeChainLink {
			return g.evaluateFastestFrontProfile(target, settings)
		}
		return g.evaluateEndToEndChainProfile(target, settings)
	default:
		return false
	}
}

func profileTypeNeedsEvaluation(profileType string) bool {
	return appevaluations.TypeNeedsEvaluation(profileType)
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
	current, err := g.evaluationRepo.CurrentConfigVersion(context.Background(), profileID)
	if err != nil {
		return 0
	}
	return current
}

func (g *Gateway) updateProfileEvaluationState(target profileEvaluationTarget, update appevaluations.StateUpdate) bool {
	return g.updateProfileEvaluationStateWithContext(context.Background(), target, update)
}

func (g *Gateway) updateProfileEvaluationStateWithContext(ctx context.Context, target profileEvaluationTarget, update appevaluations.StateUpdate) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	updated, err := g.evaluationRepo.UpdateProfileState(ctx, target.ID, target.ConfigVersion, update)
	return err == nil && updated
}

func (g *Gateway) updateProfileEvaluationStateAndReleaseRetained(target profileEvaluationTarget, keepNodeIDs []string, update appevaluations.StateUpdate) bool {
	return g.updateProfileEvaluationStateAndReleaseRetainedWithContext(context.Background(), target, keepNodeIDs, update)
}

func (g *Gateway) updateProfileEvaluationStateAndReleaseRetainedWithContext(ctx context.Context, target profileEvaluationTarget, keepNodeIDs []string, update appevaluations.StateUpdate) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := g.evaluationRepo.UpdateProfileStateAndReleaseRetained(ctx, target.ID, target.ConfigVersion, keepNodeIDs, target.NodeStickyEnabled, update)
	if err != nil || !result.Updated {
		return false
	}
	g.invalidateRuntimeFingerprints(result.DeletedFingerprints)
	return true
}

func (g *Gateway) profileLastError(profileID string) string {
	lastError, err := g.evaluationRepo.LastError(context.Background(), profileID)
	if err != nil {
		return ""
	}
	return lastError
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

func (g *Gateway) evaluateFastestProfile(target profileEvaluationTarget, settings evaluationSettings) bool {
	return g.evaluationRunService(settings).EvaluateFastest(context.Background(), target, evaluationRuntimeSettings(settings))
}

func evaluationRuntimeSettings(settings evaluationSettings) appevaluations.RuntimeSettings {
	return appevaluations.RuntimeSettings{
		GlobalConcurrency:    settings.GlobalConcurrency,
		SingleCandidateLimit: settings.SingleCandidateLimit,
		ChainCandidateLimit:  settings.ChainCandidateLimit,
	}
}

func (g *Gateway) selectedProfileNodeStillValid(target profileEvaluationTarget, nodeID string) bool {
	return g.selectedProfileNodeStillValidWithContext(context.Background(), target, nodeID)
}

func (g *Gateway) selectedProfileNodeStillValidWithContext(ctx context.Context, target profileEvaluationTarget, nodeID string) bool {
	if nodeID == "" {
		return false
	}
	node, err := g.loadNodeWithContext(ctx, nodeID)
	if err != nil || !node.Enabled {
		return false
	}
	if g.profileRetainsNodeWithContext(ctx, target.ID, nodeID) {
		return true
	}
	return g.nodeMatchesCandidateFilterWithContext(ctx, node, target.Filter)
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
	counters, err := g.evaluationRepo.CurrentPathCounters(context.Background(), profileID)
	if err != nil {
		return 0, 0
	}
	return counters.FailedEvaluations, counters.MissedSuccessCycles
}

func (g *Gateway) profileCurrentPathLatency(profileID string) int64 {
	latency, err := g.evaluationRepo.CurrentPathLatency(context.Background(), profileID)
	if err != nil {
		return 0
	}
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
	return g.profileCurrentNodeIDWithContext(context.Background(), profileID)
}

func (g *Gateway) profileCurrentNodeIDWithContext(ctx context.Context, profileID string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if g.evaluationRepo == nil {
		return ""
	}
	nodeID, err := g.evaluationRepo.CurrentNodeID(ctx, profileID)
	if err != nil {
		return ""
	}
	return nodeID
}

func (g *Gateway) candidateNodes(filter candidateFilter) ([]nodeRecord, error) {
	return g.candidateNodesWithContext(context.Background(), filter)
}

func (g *Gateway) candidateNodesWithContext(ctx context.Context, filter candidateFilter) ([]nodeRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if g.evaluationRepo == nil {
		return []nodeRecord{}, nil
	}
	records, err := appevaluations.CandidateNodes(ctx, g.evaluationRepo, g.nodeRepo.Load, filter)
	if err != nil {
		return nil, err
	}
	nodes := make([]nodeRecord, 0, len(records))
	for _, record := range records {
		nodes = append(nodes, nodeRecordFromApplication(record))
	}
	return nodes, nil
}

func (g *Gateway) nodeMatchesCandidateFilter(node nodeRecord, filter candidateFilter) bool {
	return g.nodeMatchesCandidateFilterWithContext(context.Background(), node, filter)
}

func (g *Gateway) nodeMatchesCandidateFilterWithContext(ctx context.Context, node nodeRecord, filter candidateFilter) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	if g.evaluationRepo == nil {
		return false
	}
	return appevaluations.NodeMatchesCandidateFilter(ctx, g.evaluationRepo, nodeRecordToApplication(node), filter)
}

func normalizeNodeSourceMode(mode string, sourceIDs []string, manualOnly bool) string {
	return domainprofile.NormalizeNodeSourceMode(mode, sourceIDs, manualOnly)
}

func internalNodeSourceMode(mode string) string {
	return normalizeNodeSourceMode(mode, nil, false)
}

func (g *Gateway) fetchTestURLThroughNode(node nodeRecord, testURL string, settings evaluationSettings) (int64, int, error) {
	result, err := g.probeClient().FetchThroughNode(node, testURL, settings.probeDialTimeouts())
	if err != nil {
		return 0, 0, err
	}
	return result.DurationMS, result.HTTPStatus, nil
}

func (g *Gateway) probeClient() probinginfra.Client {
	return probinginfra.Client{Engine: g.nodeEngine()}
}
