package profile

import (
	"regexp"
	"strings"
)

type CandidateFilter struct {
	EgressCountry     string
	EgressCountries   []string
	EgressCountryMode string
	NodeSourceMode    string
	SourceIDs         []string
	Protocols         []string
	NameIncludeRegex  string
	NameExcludeRegex  string
	ManualOnly        bool
}

type CandidateNode struct {
	Type        string
	Name        string
	SourceTypes []string
	SourceIDs   []string
}

type CandidateStats struct {
	Total                int
	Usable               int
	UnknownEgressCountry int
	FrontCandidates      int
	ExitNodes            int
	PathCombinations     int
}

func NormalizeNodeSourceMode(mode string, sourceIDs []string, manualOnly bool) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", NodeSourceModeAll:
		if manualOnly {
			return NodeSourceModeManual
		}
		if len(sourceIDs) > 0 {
			return NodeSourceModeSpecificSubscriptions
		}
		return NodeSourceModeAll
	case NodeSourceModeManual:
		return NodeSourceModeManual
	case NodeSourceModeAliasSubscription, NodeSourceModeSubscriptions:
		return NodeSourceModeSubscriptions
	case "specific_subscription", "specific_subscriptions", "selected_source", "selected_sources":
		return NodeSourceModeSpecificSubscriptions
	default:
		return NodeSourceModeAll
	}
}

func MatchEgressCountry(filter CandidateFilter, country string) bool {
	filter = NormalizeCandidateFilter(filter)
	if len(filter.EgressCountries) == 0 {
		return true
	}
	country = normalizeCandidateCountry(country)
	matched := stringInList(country, filter.EgressCountries)
	if filter.EgressCountryMode == EgressCountryModeExclude {
		return !matched
	}
	return matched
}

func NormalizeCandidateFilter(filter CandidateFilter) CandidateFilter {
	filter.EgressCountry = normalizeFilterCountryValue(filter.EgressCountry)
	filter.EgressCountries = normalizeFilterCountryList(filter.EgressCountries)
	if len(filter.EgressCountries) == 0 && filter.EgressCountry != "" {
		filter.EgressCountries = []string{filter.EgressCountry}
	}
	filter.EgressCountryMode = strings.ToLower(strings.TrimSpace(filter.EgressCountryMode))
	if filter.EgressCountryMode == "" {
		filter.EgressCountryMode = EgressCountryModeInclude
	}
	filter.NodeSourceMode = NormalizeNodeSourceMode(filter.NodeSourceMode, filter.SourceIDs, filter.ManualOnly)
	filter.Protocols = normalizeProtocolList(filter.Protocols)
	return filter
}

func MatchCandidateNode(filter CandidateFilter, node CandidateNode) bool {
	filter = NormalizeCandidateFilter(filter)
	node.Type = strings.ToLower(strings.TrimSpace(node.Type))
	if len(filter.Protocols) > 0 && !stringInList(node.Type, filter.Protocols) {
		return false
	}
	switch filter.NodeSourceMode {
	case NodeSourceModeManual:
		if !stringInList(NodeSourceModeManual, node.SourceTypes) {
			return false
		}
	case NodeSourceModeSubscriptions:
		if !stringInList(NodeSourceModeAliasSubscription, node.SourceTypes) {
			return false
		}
	case NodeSourceModeSpecificSubscriptions:
		if len(filter.SourceIDs) == 0 || !hasAnyString(node.SourceIDs, filter.SourceIDs) {
			return false
		}
	}
	if filter.ManualOnly && !stringInList(NodeSourceModeManual, node.SourceTypes) {
		return false
	}
	if filter.NodeSourceMode == "" && len(filter.SourceIDs) > 0 && !hasAnyString(node.SourceIDs, filter.SourceIDs) {
		return false
	}
	if filter.NameIncludeRegex != "" && !regexMatch(filter.NameIncludeRegex, node.Name) {
		return false
	}
	if filter.NameExcludeRegex != "" && regexMatch(filter.NameExcludeRegex, node.Name) {
		return false
	}
	return true
}

func BuildCandidateStats(profileType string, candidateNodeIDs []string, usableCount, unknownEgressCountryCount int, exitNodeIDs []string) CandidateStats {
	stats := CandidateStats{
		Total:                len(candidateNodeIDs),
		Usable:               usableCount,
		UnknownEgressCountry: unknownEgressCountryCount,
		PathCombinations:     len(candidateNodeIDs),
	}
	if profileType != TypeChain {
		return stats
	}
	frontCandidates := 0
	for _, nodeID := range candidateNodeIDs {
		if !stringInList(nodeID, exitNodeIDs) {
			frontCandidates++
		}
	}
	stats.FrontCandidates = frontCandidates
	stats.ExitNodes = len(exitNodeIDs)
	stats.PathCombinations = frontCandidates * len(exitNodeIDs)
	return stats
}

func normalizeFilterCountryList(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = normalizeFilterCountryValue(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizeProtocolList(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizeFilterCountryValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.EqualFold(value, "__unknown__") {
		return "__unknown__"
	}
	return strings.ToUpper(value)
}

func normalizeCandidateCountry(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "__unknown__") {
		return "__unknown__"
	}
	return strings.ToUpper(value)
}

func stringInList(value string, values []string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func hasAnyString(left, right []string) bool {
	for _, value := range left {
		if stringInList(value, right) {
			return true
		}
	}
	return false
}

func regexMatch(pattern, value string) bool {
	re, err := regexp.Compile(pattern)
	return err == nil && re.MatchString(value)
}
