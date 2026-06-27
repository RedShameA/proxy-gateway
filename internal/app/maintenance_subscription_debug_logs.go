package app

import (
	appsubscriptions "proxygateway/internal/application/subscriptions"

	"go.uber.org/zap"
)

func (g *Gateway) logSubscriptionRefreshEntrySummary(runID, subscriptionID, summaryType string, rows []appsubscriptions.SkippedEntrySummary) {
	for _, row := range rows {
		g.log().Debug("subscription refresh entry summary",
			zap.String("run_id", runID),
			zap.String("subscription_id", subscriptionID),
			zap.String("summary_type", summaryType),
			zap.String("reason", row.Reason),
			zap.Int("count", row.Count),
			zap.String("message", row.Message),
		)
	}
}

func (g *Gateway) logSubscriptionRefreshOutcome(runID string, outcome appsubscriptions.RefreshSuccessOutcome) {
	g.log().Debug("subscription refresh summary",
		zap.String("run_id", runID),
		zap.String("subscription_id", outcome.SubscriptionID),
		zap.Int("imported_count", outcome.ImportedCount),
		zap.Int("ignored_count", outcome.IgnoredCount),
		zap.Int("skipped_count", outcome.SkippedCount),
		zap.String("result", outcome.Result),
		zap.String("reason_code", outcome.ReasonCode),
	)
}
