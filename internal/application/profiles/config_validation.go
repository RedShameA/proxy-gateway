package profiles

import (
	"errors"
	"net/url"
	"regexp"
	"strings"

	domainprofile "proxygateway/internal/domain/profile"
)

var (
	ErrConfigNameRequired       = errors.New("profile name required")
	ErrIdentifierDuplicate      = errors.New("profile identifier duplicate")
	ErrFixedNodeRequired        = errors.New("fixed node required")
	ErrFixedNodeNotFound        = errors.New("fixed node not found")
	ErrExitNodeNotFound         = errors.New("exit node not found")
	ErrTestURLScheme            = errors.New("test url scheme")
	ErrTestURLHostRequired      = errors.New("test url host required")
	ErrNameIncludeRegexInvalid  = errors.New("name include regex invalid")
	ErrNameExcludeRegexInvalid  = errors.New("name exclude regex invalid")
	ErrEgressCountryModeInvalid = errors.New("egress country mode invalid")
	ErrSelectedSourcesRequired  = errors.New("selected sources required")
	ErrUnsupportedProfileType   = errors.New("unsupported access profile type")
)

type ConfigValidationDeps struct {
	DefaultTestURL   string
	IdentifierExists func(identifier, excludeProfileID string) (bool, error)
	NodeExists       func(nodeID string) (bool, error)
}

func ValidateConfig(record *ConfigRecord, deps ConfigValidationDeps) error {
	record.ApplyDefaults()
	record.Name = strings.TrimSpace(record.Name)
	if record.Name == "" {
		return ErrConfigNameRequired
	}
	record.ProfileIdentifier = strings.TrimSpace(record.ProfileIdentifier)
	if record.ProfileIdentifier == "" {
		record.ProfileIdentifier = record.ID
	}
	if err := domainprofile.ValidateIdentifier(record.ProfileIdentifier); err != nil {
		return err
	}
	if deps.IdentifierExists != nil {
		exists, err := deps.IdentifierExists(record.ProfileIdentifier, record.ID)
		if err != nil {
			return err
		}
		if exists {
			return ErrIdentifierDuplicate
		}
	}
	if err := domainprofile.ValidateEvaluationTiming(record.CandidateLimit, record.MinEvaluationIntervalSeconds, record.AutoEvaluationInterval); err != nil {
		return err
	}
	if err := domainprofile.ValidateSwitchingTolerance(record.RelativeImprovementThreshold, record.AbsoluteLatencyImprovementMS); err != nil {
		return err
	}

	record.FixedNodeID = strings.TrimSpace(record.FixedNodeID)
	record.TestURL = strings.TrimSpace(record.TestURL)
	record.NameIncludeRegex = strings.TrimSpace(record.NameIncludeRegex)
	record.NameExcludeRegex = strings.TrimSpace(record.NameExcludeRegex)
	if err := validateOptionalRegex(record.NameIncludeRegex, ErrNameIncludeRegexInvalid); err != nil {
		return err
	}
	if err := validateOptionalRegex(record.NameExcludeRegex, ErrNameExcludeRegexInvalid); err != nil {
		return err
	}

	record.ExitNodeIDs = normalizeStringList(record.ExitNodeIDs)
	record.EgressCountries = normalizeEgressCountryList(record.EgressCountries)
	record.Protocols = normalizeLowerStringList(record.Protocols)
	record.EgressCountry = normalizeEgressCountryValue(record.EgressCountry)
	if len(record.EgressCountries) == 0 && record.EgressCountry != "" {
		record.EgressCountries = []string{record.EgressCountry}
	}
	if len(record.EgressCountries) > 0 {
		record.EgressCountry = record.EgressCountries[0]
	} else {
		record.EgressCountry = ""
	}
	record.EgressCountryMode = strings.ToLower(strings.TrimSpace(record.EgressCountryMode))
	if record.EgressCountryMode == "" {
		record.EgressCountryMode = "include"
	}
	if record.EgressCountryMode != "include" && record.EgressCountryMode != "exclude" {
		return ErrEgressCountryModeInvalid
	}
	record.NodeSourceMode = domainprofile.NormalizeNodeSourceMode(record.NodeSourceMode, record.SourceIDs, record.ManualOnly)
	if record.NodeSourceMode == "specific_subscriptions" && len(record.SourceIDs) == 0 {
		return ErrSelectedSourcesRequired
	}
	record.ManualOnly = record.NodeSourceMode == "manual"
	record.Type = strings.TrimSpace(record.Type)

	switch record.Type {
	case "fixed_node":
		if record.FixedNodeID == "" {
			return ErrFixedNodeRequired
		}
		if !nodeExists(record.FixedNodeID, deps.NodeExists) {
			return ErrFixedNodeNotFound
		}
		record.CurrentNodeID = record.FixedNodeID
		record.CurrentExitNodeID = ""
		record.ExitNodeIDs = []string{}
		record.ChainEvaluationMode = ""
		record.NodeStickyEnabled = false
		record.State = "ready"
	case "fastest":
		if err := validateProfileTestURL(record.TestURL, deps.DefaultTestURL); err != nil {
			return err
		}
		record.TestURL = EffectiveTestURL(record.TestURL, deps.DefaultTestURL)
		record.CurrentExitNodeID = ""
		record.State = record.DynamicStateAfterUpdate()
	case "random":
		record.CurrentNodeID = ""
		record.CurrentExitNodeID = ""
		record.NodeStickyEnabled = false
		record.State = "ready"
	case "chain":
		if len(record.ExitNodeIDs) == 0 && record.FixedNodeID != "" {
			record.ExitNodeIDs = []string{record.FixedNodeID}
		}
		if err := domainprofile.ValidateChainExitNodes(record.ExitNodeIDs, "end_to_end"); err != nil {
			return err
		}
		for _, exitNodeID := range record.ExitNodeIDs {
			if !nodeExists(exitNodeID, deps.NodeExists) {
				return ErrExitNodeNotFound
			}
		}
		record.FixedNodeID = record.ExitNodeIDs[0]
		record.ChainEvaluationMode = domainprofile.NormalizeChainEvaluationMode(record.ChainEvaluationMode)
		if err := domainprofile.ValidateChainExitNodes(record.ExitNodeIDs, record.ChainEvaluationMode); err != nil {
			return err
		}
		if record.ChainEvaluationMode == "end_to_end" {
			if err := validateProfileTestURL(record.TestURL, deps.DefaultTestURL); err != nil {
				return err
			}
			record.TestURL = EffectiveTestURL(record.TestURL, deps.DefaultTestURL)
		}
		if !stringInList(record.CurrentExitNodeID, record.ExitNodeIDs) {
			record.CurrentExitNodeID = ""
		}
		if record.CurrentNodeID != "" && record.CurrentExitNodeID == "" && len(record.ExitNodeIDs) == 1 {
			record.CurrentExitNodeID = record.ExitNodeIDs[0]
		}
		record.State = record.DynamicStateAfterUpdate()
	default:
		return ErrUnsupportedProfileType
	}
	return nil
}

func EffectiveTestURL(testURL, defaultTestURL string) string {
	testURL = strings.TrimSpace(testURL)
	if testURL == "" {
		return strings.TrimSpace(defaultTestURL)
	}
	if !strings.Contains(testURL, "://") {
		return "https://" + testURL
	}
	return testURL
}

func validateProfileTestURL(testURL, defaultTestURL string) error {
	effective := EffectiveTestURL(testURL, defaultTestURL)
	u, err := url.Parse(effective)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrTestURLScheme
	}
	if strings.TrimSpace(u.Host) == "" {
		return ErrTestURLHostRequired
	}
	return nil
}

func validateOptionalRegex(pattern string, err error) error {
	if pattern == "" {
		return nil
	}
	if _, compileErr := regexp.Compile(pattern); compileErr != nil {
		return err
	}
	return nil
}

func nodeExists(nodeID string, exists func(string) (bool, error)) bool {
	if exists == nil {
		return true
	}
	ok, err := exists(nodeID)
	return err == nil && ok
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

func normalizeEgressCountryList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeEgressCountryValue(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalizeStringList(normalized)
}

func normalizeLowerStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalizeStringList(normalized)
}

func stringInList(value string, values []string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
