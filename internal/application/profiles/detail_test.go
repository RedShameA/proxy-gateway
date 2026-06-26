package profiles

import "testing"

func TestBuildDetailAppliesStableReadModelDefaults(t *testing.T) {
	detail := BuildDetail(DetailInput{
		Summary: SummaryInput{
			ID:                    "profile_1",
			Name:                  "chain",
			Type:                  "chain",
			State:                 "ready",
			ProfileIdentifier:     "chain-main",
			AutoEvaluationEnabled: true,
			ConfigVersion:         4,
		},
		FixedNodeID:                  "exit_1",
		ExitNodeIDs:                  nil,
		ChainEvaluationMode:          " chain_link ",
		CurrentPathLatencyMS:         0,
		LastEvaluationDetails:        nil,
		ProxyCredentials:             nil,
		CandidateStats:               CandidateStats{Total: 2, Usable: 1, FrontCandidates: 1, ExitNodes: 1, PathCombinations: 1},
		RecentEvents:                 nil,
		CandidateFilterSourceMode:    "selected_sources",
		RelativeImprovementThreshold: 0.2,
		AbsoluteLatencyImprovementMS: 100,
	})

	if detail.FixedNodeID != "exit_1" {
		t.Fatalf("FixedNodeID = %#v, want exit_1", detail.FixedNodeID)
	}
	if detail.ChainEvaluationMode != "chain_link" {
		t.Fatalf("ChainEvaluationMode = %#v, want chain_link", detail.ChainEvaluationMode)
	}
	if detail.CurrentPathLatencyMS != nil {
		t.Fatalf("CurrentPathLatencyMS = %#v, want nil", detail.CurrentPathLatencyMS)
	}
	if len(detail.ExitNodeIDs) != 0 || detail.ExitNodeIDs == nil {
		t.Fatalf("ExitNodeIDs = %#v, want empty slice", detail.ExitNodeIDs)
	}
	if len(detail.ProxyCredentials) != 0 || detail.ProxyCredentials == nil {
		t.Fatalf("ProxyCredentials = %#v, want empty slice", detail.ProxyCredentials)
	}
	if len(detail.RecentEvents) != 0 || detail.RecentEvents == nil {
		t.Fatalf("RecentEvents = %#v, want empty slice", detail.RecentEvents)
	}
	if detail.EvaluationSchedule.Mode != "custom" {
		t.Fatalf("EvaluationSchedule.Mode = %q, want custom", detail.EvaluationSchedule.Mode)
	}
	if detail.CandidateStats.PathCombinations != 1 {
		t.Fatalf("PathCombinations = %d, want 1", detail.CandidateStats.PathCombinations)
	}
	if detail.CandidateFilter.SourceMode != "selected_sources" {
		t.Fatalf("CandidateFilter.SourceMode = %q, want selected_sources", detail.CandidateFilter.SourceMode)
	}
}
