package evaluations

type CandidateCountInput struct {
	Target               Target
	CandidateNodeIDs     []string
	SingleCandidateLimit int
}

func ProfileEvaluationCandidateCount(input CandidateCountInput) int {
	switch input.Target.Type {
	case "chain":
		return chainCandidateCount(input.CandidateNodeIDs, input.Target.ExitNodeIDs)
	case "fastest":
		return limitedCandidateCount(len(input.CandidateNodeIDs), effectiveCandidateLimit(input.Target.CandidateLimit, input.SingleCandidateLimit))
	default:
		return 0
	}
}

func chainCandidateCount(candidateNodeIDs, exitNodeIDs []string) int {
	frontCount := 0
	for _, nodeID := range candidateNodeIDs {
		if stringInList(nodeID, exitNodeIDs) {
			continue
		}
		frontCount++
	}
	return frontCount * len(exitNodeIDs)
}

func effectiveCandidateLimit(profileLimit, settingsLimit int) int {
	if profileLimit > 0 {
		return profileLimit
	}
	return settingsLimit
}

func limitedCandidateCount(count, limit int) int {
	if limit <= 0 || count <= limit {
		return count
	}
	return limit
}

func stringInList(value string, values []string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
