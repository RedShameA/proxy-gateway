package evaluations

import "math"

type ChainProbeResult struct {
	FrontNodeID string
	ExitNodeID  string
	DurationMS  int64
	OK          bool
	Error       string
}

type ChainProbeSummary struct {
	BestFrontNodeID   string
	BestExitNodeID    string
	BestDurationMS    int64
	CurrentDurationMS int64
	FailureCount      int
	LastFailure       string
}

func SummarizeChainProbeResults(currentFrontNodeID, currentExitNodeID string, results []ChainProbeResult) ChainProbeSummary {
	summary := ChainProbeSummary{
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
		if result.FrontNodeID == currentFrontNodeID && result.ExitNodeID == currentExitNodeID {
			summary.CurrentDurationMS = result.DurationMS
		}
		if result.DurationMS < summary.BestDurationMS {
			summary.BestDurationMS = result.DurationMS
			summary.BestFrontNodeID = result.FrontNodeID
			summary.BestExitNodeID = result.ExitNodeID
		}
	}
	return summary
}
