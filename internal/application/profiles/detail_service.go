package profiles

import (
	"context"

	appreadmodel "proxygateway/internal/application/readmodel"
	domainprofile "proxygateway/internal/domain/profile"
)

type CandidateNodeIDs func(filter domainprofile.CandidateFilter) ([]string, error)
type NodeUsableLookup func(nodeID string) bool
type UnknownCountryCandidateCounter func(filter domainprofile.CandidateFilter) int
type CurrentPathBuilder func(record ConfigRecord) any
type RecentEventsLoader func(ctx context.Context, profileID string, limit int) ([]map[string]any, error)

type DetailServiceDeps struct {
	Configs                      ConfigRepository
	Credentials                  CredentialRepository
	CandidateNodeIDs             CandidateNodeIDs
	NodeUsable                   NodeUsableLookup
	UnknownCountryCandidateCount UnknownCountryCandidateCounter
	CurrentPath                  CurrentPathBuilder
	RecentEvents                 RecentEventsLoader
}

type DetailService struct {
	deps DetailServiceDeps
}

func NewDetailService(deps DetailServiceDeps) DetailService {
	return DetailService{deps: deps}
}

func (s DetailService) Load(ctx context.Context, profileID, endpoint string) (Detail, error) {
	cfg, found, err := s.deps.Configs.LoadConfig(ctx, profileID)
	if err != nil {
		return Detail{}, err
	}
	if !found {
		return Detail{}, ErrProfileNotFound
	}
	cfg.ApplyDefaults()
	sourceIDs := nonNilStrings(cfg.SourceIDs)
	profileIdentifier := cfg.EffectiveProfileIdentifier()

	credentials := []Credential{}
	counts := CredentialCounts{}
	if s.deps.Credentials != nil {
		if records, err := s.deps.Credentials.ListCredentials(ctx, profileID); err == nil {
			credentials = BuildCredentials(records, profileIdentifier, endpoint)
		}
		counts, _ = s.deps.Credentials.CountCredentials(ctx, profileID)
	}

	filter := cfg.CandidateFilter()
	candidateNodeIDs, usableCount := s.candidateNodeStats(filter)
	egressCountries := nonNilStrings(cfg.EgressCountries)
	protocols := nonNilStrings(cfg.Protocols)
	exitNodeIDs := nonNilStrings(cfg.ExitNodeIDs)
	summaryCfg := cfg
	summaryCfg.SourceIDs = sourceIDs
	summaryCfg.EgressCountries = egressCountries

	return BuildDetail(DetailInput{
		Summary:                      SummaryInputFromConfig(summaryCfg, s.currentPath(cfg), counts),
		FixedNodeID:                  cfg.FixedNodeID,
		ExitNodeIDs:                  exitNodeIDs,
		ChainEvaluationMode:          cfg.ChainEvaluationMode,
		TestURL:                      cfg.TestURL,
		CurrentPathLatencyMS:         cfg.CurrentPathLatencyMS,
		LastEvaluationDetails:        appreadmodel.ParseJSONObject(cfg.LastEvaluationDetailsJSON),
		ProxyCredentials:             credentials,
		CandidateStats:               BuildCandidateStats(cfg.Type, candidateNodeIDs, usableCount, s.unknownCountryCandidateCount(filter), cfg.ExitNodeIDs),
		RecentEvents:                 s.recentEvents(ctx, profileID),
		CandidateFilterSourceMode:    APINodeSourceMode(cfg.NodeSourceMode),
		SourceIDs:                    sourceIDs,
		Protocols:                    protocols,
		EgressCountries:              egressCountries,
		RelativeImprovementThreshold: cfg.RelativeImprovementThreshold,
		AbsoluteLatencyImprovementMS: cfg.AbsoluteLatencyImprovementMS,
	}), nil
}

func (s DetailService) candidateNodeStats(filter domainprofile.CandidateFilter) ([]string, int) {
	if s.deps.CandidateNodeIDs == nil {
		return nil, 0
	}
	nodeIDs, err := s.deps.CandidateNodeIDs(filter)
	if err != nil {
		return nil, 0
	}
	usableCount := 0
	for _, nodeID := range nodeIDs {
		if s.deps.NodeUsable != nil && s.deps.NodeUsable(nodeID) {
			usableCount++
		}
	}
	return nodeIDs, usableCount
}

func (s DetailService) unknownCountryCandidateCount(filter domainprofile.CandidateFilter) int {
	if s.deps.UnknownCountryCandidateCount == nil {
		return 0
	}
	return s.deps.UnknownCountryCandidateCount(filter)
}

func (s DetailService) currentPath(cfg ConfigRecord) any {
	if s.deps.CurrentPath == nil {
		return nil
	}
	return s.deps.CurrentPath(cfg)
}

func (s DetailService) recentEvents(ctx context.Context, profileID string) []map[string]any {
	if s.deps.RecentEvents == nil {
		return []map[string]any{}
	}
	events, err := s.deps.RecentEvents(ctx, profileID, 10)
	if err != nil || events == nil {
		return []map[string]any{}
	}
	return events
}

func APINodeSourceMode(mode string) string {
	switch domainprofile.NormalizeNodeSourceMode(mode, nil, false) {
	case "subscriptions":
		return "subscription"
	case "specific_subscriptions":
		return "selected_sources"
	case "manual":
		return "manual"
	default:
		return "all"
	}
}
