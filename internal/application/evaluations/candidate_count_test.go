package evaluations

import "testing"

func TestProfileEvaluationCandidateCountLimitsFastestCandidates(t *testing.T) {
	target := Target{Type: "fastest"}
	count := ProfileEvaluationCandidateCount(CandidateCountInput{
		Target:               target,
		CandidateNodeIDs:     []string{"a", "b", "c"},
		SingleCandidateLimit: 2,
	})
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	target.CandidateLimit = 1
	count = ProfileEvaluationCandidateCount(CandidateCountInput{
		Target:               target,
		CandidateNodeIDs:     []string{"a", "b", "c"},
		SingleCandidateLimit: 2,
	})
	if count != 1 {
		t.Fatalf("profile limit count = %d, want 1", count)
	}
}

func TestProfileEvaluationCandidateCountCountsChainCombinations(t *testing.T) {
	count := ProfileEvaluationCandidateCount(CandidateCountInput{
		Target: Target{
			Type:        "chain",
			ExitNodeIDs: []string{"exit_1", "exit_2"},
		},
		CandidateNodeIDs: []string{"front_1", "exit_1", "front_2"},
	})
	if count != 4 {
		t.Fatalf("count = %d, want 4", count)
	}
}

func TestProfileEvaluationCandidateCountIgnoresUnsupportedTypes(t *testing.T) {
	count := ProfileEvaluationCandidateCount(CandidateCountInput{
		Target:           Target{Type: "random"},
		CandidateNodeIDs: []string{"a", "b"},
	})
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}
