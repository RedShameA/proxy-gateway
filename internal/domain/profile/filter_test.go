package profile

import "testing"

func TestNormalizeNodeSourceMode(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		sourceIDs  []string
		manualOnly bool
		want       string
	}{
		{name: "default all", want: "all"},
		{name: "manual only legacy", manualOnly: true, want: "manual"},
		{name: "selected sources implicit", sourceIDs: []string{"sub-1"}, want: "specific_subscriptions"},
		{name: "subscription alias", mode: "subscription", want: "subscriptions"},
		{name: "selected sources alias", mode: "selected_sources", want: "specific_subscriptions"},
		{name: "manual explicit", mode: "manual", want: "manual"},
		{name: "unknown fallback", mode: "weird", want: "all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeNodeSourceMode(tt.mode, tt.sourceIDs, tt.manualOnly); got != tt.want {
				t.Fatalf("NormalizeNodeSourceMode(%q, %#v, %v) = %q, want %q", tt.mode, tt.sourceIDs, tt.manualOnly, got, tt.want)
			}
		})
	}
}

func TestMatchEgressCountry(t *testing.T) {
	filter := CandidateFilter{
		EgressCountryMode: "include",
		EgressCountries:   []string{"JP", "__unknown__"},
	}
	if !MatchEgressCountry(filter, "jp") {
		t.Fatal("JP should match include filter")
	}
	if !MatchEgressCountry(filter, "") {
		t.Fatal("blank country should match explicit __unknown__ include filter")
	}
	if MatchEgressCountry(filter, "US") {
		t.Fatal("US should not match include filter")
	}

	exclude := CandidateFilter{
		EgressCountryMode: "exclude",
		EgressCountries:   []string{"__unknown__"},
	}
	if MatchEgressCountry(exclude, "") {
		t.Fatal("blank country should be excluded when __unknown__ is excluded")
	}
	if !MatchEgressCountry(exclude, "US") {
		t.Fatal("US should pass exclude filter")
	}
}

func TestNormalizeCandidateFilterDoesNotTreatBlankCountryAsUnknownFilter(t *testing.T) {
	filter := NormalizeCandidateFilter(CandidateFilter{})
	if filter.EgressCountry != "" {
		t.Fatalf("EgressCountry = %q, want empty", filter.EgressCountry)
	}
	if len(filter.EgressCountries) != 0 {
		t.Fatalf("EgressCountries = %#v, want empty", filter.EgressCountries)
	}
	if filter.EgressCountryMode != "include" {
		t.Fatalf("EgressCountryMode = %q, want include", filter.EgressCountryMode)
	}
}

func TestMatchCandidateNode(t *testing.T) {
	node := CandidateNode{
		Type:        "http",
		Name:        "allowed-manual-jp",
		SourceTypes: []string{"manual"},
		SourceIDs:   []string{"manual-source"},
	}
	filter := CandidateFilter{
		NodeSourceMode:   "manual",
		Protocols:        []string{"http"},
		NameIncludeRegex: "allowed",
		NameExcludeRegex: "blocked",
	}
	if !MatchCandidateNode(filter, node) {
		t.Fatal("manual http node should match")
	}
	if MatchCandidateNode(CandidateFilter{NodeSourceMode: "subscriptions"}, node) {
		t.Fatal("manual node must not match subscription filter")
	}
	if MatchCandidateNode(CandidateFilter{Protocols: []string{"socks5"}}, node) {
		t.Fatal("http node must not match socks5 filter")
	}
	if MatchCandidateNode(CandidateFilter{NameExcludeRegex: "manual"}, node) {
		t.Fatal("exclude regex should reject matching node")
	}

	selected := CandidateFilter{SourceIDs: []string{"sub-1"}}
	subNode := CandidateNode{
		Type:        "http",
		Name:        "subscription-node",
		SourceTypes: []string{"subscription"},
		SourceIDs:   []string{"sub-1", "sub-2"},
	}
	if !MatchCandidateNode(selected, subNode) {
		t.Fatal("matching subscription source should pass")
	}
	if MatchCandidateNode(CandidateFilter{SourceIDs: []string{"sub-9"}}, subNode) {
		t.Fatal("non-overlapping selected source should fail")
	}
}

func TestBuildCandidateStats(t *testing.T) {
	single := BuildCandidateStats("fastest", []string{"a", "b"}, 1, 1, nil)
	if single.Total != 2 || single.Usable != 1 || single.UnknownEgressCountry != 1 || single.FrontCandidates != 0 || single.ExitNodes != 0 || single.PathCombinations != 2 {
		t.Fatalf("single stats = %#v", single)
	}

	chain := BuildCandidateStats("chain", []string{"front-1", "exit-1", "front-2"}, 2, 1, []string{"exit-1", "exit-2"})
	if chain.Total != 3 || chain.Usable != 2 || chain.UnknownEgressCountry != 1 {
		t.Fatalf("chain stats base counts = %#v", chain)
	}
	if chain.FrontCandidates != 2 || chain.ExitNodes != 2 || chain.PathCombinations != 4 {
		t.Fatalf("chain combinatorics = %#v, want front=2 exit=2 path=4", chain)
	}
}
