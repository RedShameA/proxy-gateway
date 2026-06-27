package app

import (
	appevaluations "proxygateway/internal/application/evaluations"

	"go.uber.org/zap"
)

type profileEvaluationSelectionLog struct {
	ProfileID          string
	ProfileType        string
	CandidateCount     int
	FailureCount       int
	BestNodeID         string
	BestExitNodeID     string
	CurrentNodeID      string
	CurrentExitNodeID  string
	SelectedNodeID     string
	SelectedExitNodeID string
	SwitchReason       string
}

func (g *Gateway) logFastestCandidateProbe(profileID string, result profileCandidateProbeResult) {
	fields := []zap.Field{
		zap.String("profile_id", profileID),
		zap.String("node_id", result.Node.ID),
		zap.String("node_name", result.Node.Name),
		zap.Bool("success", result.succeeded()),
		zap.Int64("duration_ms", result.Duration),
	}
	fields = appendHTTPStatusField(fields, result.Status)
	fields = append(fields, zap.String("error", profileCandidateProbeError(result)))
	g.log().Debug("profile candidate probe result", fields...)
}

func (g *Gateway) logChainCandidateProbe(profileID string, result chainCandidateProbeResult) {
	fields := []zap.Field{
		zap.String("profile_id", profileID),
		zap.String("front_node_id", result.Pair.FrontNode.ID),
		zap.String("front_node_name", result.Pair.FrontNode.Name),
		zap.String("exit_node_id", result.Pair.ExitNode.ID),
		zap.String("exit_node_name", result.Pair.ExitNode.Name),
		zap.Bool("success", result.succeeded()),
		zap.Int64("duration_ms", result.Duration),
	}
	fields = appendHTTPStatusField(fields, result.Status)
	fields = append(fields, zap.String("error", chainCandidateProbeError(result)))
	g.log().Debug("chain candidate probe result", fields...)
}

func appendHTTPStatusField(fields []zap.Field, status int) []zap.Field {
	if status > 0 {
		fields = append(fields, zap.Int("http_status", status))
	}
	return fields
}

func profileCandidateProbeError(result profileCandidateProbeResult) string {
	if result.Err == nil {
		return ""
	}
	return result.Err.Error()
}

func chainCandidateProbeError(result chainCandidateProbeResult) string {
	if result.Err == nil {
		return ""
	}
	return result.Err.Error()
}

func (g *Gateway) logFastestEvaluationSelection(target profileEvaluationTarget, candidateCount int, summary appevaluations.FastestProbeSummary, currentNodeID, selectedNodeID, switchReason string) {
	g.logProfileEvaluationSelection(profileEvaluationSelectionLog{
		ProfileID:      target.ID,
		ProfileType:    target.Type,
		CandidateCount: candidateCount,
		FailureCount:   summary.FailureCount,
		BestNodeID:     summary.BestNodeID,
		CurrentNodeID:  currentNodeID,
		SelectedNodeID: selectedNodeID,
		SwitchReason:   switchReason,
	})
}

func (g *Gateway) logChainEvaluationSelection(target profileEvaluationTarget, candidateCount int, summary appevaluations.ChainProbeSummary, currentNodeID, currentExitNodeID, selectedNodeID, selectedExitNodeID, switchReason string) {
	g.logProfileEvaluationSelection(profileEvaluationSelectionLog{
		ProfileID:          target.ID,
		ProfileType:        target.Type,
		CandidateCount:     candidateCount,
		FailureCount:       summary.FailureCount,
		BestNodeID:         summary.BestFrontNodeID,
		BestExitNodeID:     summary.BestExitNodeID,
		CurrentNodeID:      currentNodeID,
		CurrentExitNodeID:  currentExitNodeID,
		SelectedNodeID:     selectedNodeID,
		SelectedExitNodeID: selectedExitNodeID,
		SwitchReason:       switchReason,
	})
}

func (g *Gateway) logProfileEvaluationSelection(selection profileEvaluationSelectionLog) {
	g.log().Debug("profile evaluation selected path",
		zap.String("profile_id", selection.ProfileID),
		zap.String("profile_type", selection.ProfileType),
		zap.Int("candidate_count", selection.CandidateCount),
		zap.Int("failure_count", selection.FailureCount),
		zap.String("best_node_id", selection.BestNodeID),
		zap.String("best_exit_node_id", selection.BestExitNodeID),
		zap.String("current_node_id", selection.CurrentNodeID),
		zap.String("current_exit_node_id", selection.CurrentExitNodeID),
		zap.String("selected_node_id", selection.SelectedNodeID),
		zap.String("selected_exit_node_id", selection.SelectedExitNodeID),
		zap.String("switch_reason", selection.SwitchReason),
	)
}
