package app

import (
	appobservations "proxygateway/internal/application/observations"

	"go.uber.org/zap"
)

func (g *Gateway) logNodeObservationResult(runID string, result appobservations.RunResult) {
	g.log().Debug("node observation result",
		zap.String("run_id", runID),
		zap.String("node_id", result.NodeID),
		zap.String("node_name", result.Name),
		zap.Bool("success", result.OK),
		zap.String("error", result.Error),
	)
}
