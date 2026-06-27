package evaluations

import "testing"

func TestBuildTargetNormalizesEvaluationTarget(t *testing.T) {
	target, skipped := BuildTarget(TargetBuildInput{
		Record: TargetRecord{
			ID:                           "profile_1",
			Type:                         "chain",
			FixedNodeID:                  "exit_1",
			ChainEvaluationMode:          " chain_link ",
			TestURL:                      "example.com",
			EgressCountry:                "jp",
			EgressCountryMode:            "",
			NodeSourceMode:               "selected_sources",
			SourceIDs:                    []string{"sub_1"},
			Protocols:                    []string{"vmess"},
			NameIncludeRegex:             "tokyo",
			NameExcludeRegex:             "slow",
			CandidateLimit:               10,
			MinEvaluationIntervalSeconds: 60,
			ConfigVersion:                7,
			RelativeImprovementThreshold: 0.2,
			AbsoluteImprovementMS:        100,
			NodeStickyEnabled:            true,
		},
		DefaultTestURL:                      "https://www.gstatic.com/generate_204",
		DefaultMinEvaluationIntervalSeconds: 300,
		NowMS:                               1_000,
		ForceSwitch:                         true,
	})

	if skipped {
		t.Fatal("skipped = true, want false")
	}
	if target.ID != "profile_1" || target.Type != "chain" || target.ConfigVersion != 7 {
		t.Fatalf("target identity = %#v", target)
	}
	if target.TestURL != "https://example.com" {
		t.Fatalf("TestURL = %q", target.TestURL)
	}
	if len(target.ExitNodeIDs) != 1 || target.ExitNodeIDs[0] != "exit_1" {
		t.Fatalf("ExitNodeIDs = %#v", target.ExitNodeIDs)
	}
	if target.ChainEvaluationMode != "chain_link" {
		t.Fatalf("ChainEvaluationMode = %q", target.ChainEvaluationMode)
	}
	if target.Filter.EgressCountryMode != "include" || target.Filter.EgressCountries[0] != "JP" {
		t.Fatalf("filter countries = %#v", target.Filter)
	}
	if target.Filter.NodeSourceMode != "specific_subscriptions" || target.Filter.SourceIDs[0] != "sub_1" {
		t.Fatalf("filter sources = %#v", target.Filter)
	}
	if !target.ForceSwitch || !target.NodeStickyEnabled {
		t.Fatalf("target flags = %#v", target)
	}
}

func TestBuildTargetSkipsWhenMinIntervalHasNotElapsed(t *testing.T) {
	_, skipped := BuildTarget(TargetBuildInput{
		Record: TargetRecord{
			ID:              "profile_1",
			Type:            "fastest",
			LastEvaluatedAt: 1_000,
		},
		DefaultTestURL:                      "https://www.gstatic.com/generate_204",
		DefaultMinEvaluationIntervalSeconds: 300,
		NowMS:                               1_000 + 299_000,
	})

	if !skipped {
		t.Fatal("skipped = false, want true")
	}
}

func TestTypeNeedsEvaluation(t *testing.T) {
	if !TypeNeedsEvaluation("fastest") || !TypeNeedsEvaluation("chain") {
		t.Fatal("dynamic profile types should need evaluation")
	}
	if TypeNeedsEvaluation("fixed_node") || TypeNeedsEvaluation("random") {
		t.Fatal("static/random profile types should not need evaluation")
	}
}
