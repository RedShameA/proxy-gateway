package evaluations

import "strings"

type TargetSnapshotInput struct {
	FixedNodeID                         string
	ExitNodeIDs                         []string
	TestURL                             string
	DefaultTestURL                      string
	LastEvaluatedAt                     int64
	MinEvaluationIntervalSeconds        int
	DefaultMinEvaluationIntervalSeconds int
	NowMS                               int64
	ForceSwitch                         bool
}

type TargetSnapshot struct {
	ExitNodeIDs                 []string
	TestURL                     string
	EffectiveMinIntervalSeconds int
}

func BuildTargetSnapshot(input TargetSnapshotInput) (TargetSnapshot, bool) {
	exitNodeIDs := normalizeStringList(input.ExitNodeIDs)
	if len(exitNodeIDs) == 0 && strings.TrimSpace(input.FixedNodeID) != "" {
		exitNodeIDs = []string{strings.TrimSpace(input.FixedNodeID)}
	}
	effectiveMinInterval := input.MinEvaluationIntervalSeconds
	if effectiveMinInterval == 0 {
		effectiveMinInterval = input.DefaultMinEvaluationIntervalSeconds
	}
	snapshot := TargetSnapshot{
		ExitNodeIDs:                 exitNodeIDs,
		TestURL:                     effectiveProfileTestURL(input.TestURL, input.DefaultTestURL),
		EffectiveMinIntervalSeconds: effectiveMinInterval,
	}
	if !input.ForceSwitch && input.LastEvaluatedAt > 0 && effectiveMinInterval > 0 && input.NowMS-input.LastEvaluatedAt < secondsToMillis(int64(effectiveMinInterval)) {
		return snapshot, true
	}
	return snapshot, false
}

func effectiveProfileTestURL(testURL, defaultTestURL string) string {
	if trimmed := strings.TrimSpace(testURL); trimmed != "" {
		if !strings.Contains(trimmed, "://") {
			return "https://" + trimmed
		}
		return trimmed
	}
	return strings.TrimSpace(defaultTestURL)
}

func normalizeStringList(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func secondsToMillis(seconds int64) int64 {
	return seconds * 1000
}
