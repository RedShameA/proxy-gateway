package evaluations

import "math"

type FastestProbeResult struct {
	NodeID     string
	DurationMS int64
	OK         bool
	Error      string
}

type FastestProbeSummary struct {
	BestNodeID        string
	BestDurationMS    int64
	CurrentDurationMS int64
	FailureCount      int
	LastFailure       string
}

func SummarizeFastestProbeResults(currentNodeID string, results []FastestProbeResult) FastestProbeSummary {
	summary := FastestProbeSummary{
		BestDurationMS:    int64(math.MaxInt64),
		CurrentDurationMS: int64(math.MaxInt64),
	}
	for _, result := range results {
		if !result.OK {
			summary.FailureCount++
			if result.Error != "" {
				summary.LastFailure = result.Error
			} else {
				summary.LastFailure = "test url probe failed without error detail"
			}
			continue
		}
		if result.NodeID == currentNodeID {
			summary.CurrentDurationMS = result.DurationMS
		}
		if result.DurationMS < summary.BestDurationMS {
			summary.BestDurationMS = result.DurationMS
			summary.BestNodeID = result.NodeID
		}
	}
	return summary
}
