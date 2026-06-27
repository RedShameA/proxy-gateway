package evaluations

import (
	"context"
	"strings"

	appnodes "proxygateway/internal/application/nodes"
	domainprofile "proxygateway/internal/domain/profile"
)

type CandidateRepository interface {
	ListCandidateNodeIDs(ctx context.Context) ([]string, error)
	CandidateEgressCountry(ctx context.Context, nodeID string) (string, bool, error)
	ListCandidateSourceRefs(ctx context.Context, nodeID string) ([]SourceRef, error)
}

type CandidateNodeLoader func(ctx context.Context, nodeID string) (appnodes.Record, bool, error)

func CandidateNodes(ctx context.Context, repo CandidateRepository, load CandidateNodeLoader, filter domainprofile.CandidateFilter) ([]appnodes.Record, error) {
	filter = domainprofile.NormalizeCandidateFilter(filter)
	nodeIDs, err := repo.ListCandidateNodeIDs(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]appnodes.Record, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		if len(filter.EgressCountries) > 0 && !candidateEgressCountryMatches(ctx, repo, nodeID, filter) {
			continue
		}
		node, found, err := load(ctx, nodeID)
		if err != nil || !found {
			continue
		}
		if NodeMatchesCandidateFilter(ctx, repo, node, filter) {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

func NodeMatchesCandidateFilter(ctx context.Context, repo CandidateRepository, node appnodes.Record, filter domainprofile.CandidateFilter) bool {
	return domainprofile.MatchCandidateNode(filter, loadCandidateNode(ctx, repo, node, filter))
}

func candidateEgressCountryMatches(ctx context.Context, repo CandidateRepository, nodeID string, filter domainprofile.CandidateFilter) bool {
	country, found, err := repo.CandidateEgressCountry(ctx, nodeID)
	if err != nil || !found || strings.TrimSpace(country) == "" {
		country = ""
	}
	return domainprofile.MatchEgressCountry(filter, country)
}

func loadCandidateNode(ctx context.Context, repo CandidateRepository, node appnodes.Record, filter domainprofile.CandidateFilter) domainprofile.CandidateNode {
	candidate := domainprofile.CandidateNode{
		Type: node.Type,
		Name: node.Name,
	}
	filter = domainprofile.NormalizeCandidateFilter(filter)
	if filter.NodeSourceMode == "all" && !filter.ManualOnly && len(filter.SourceIDs) == 0 {
		return candidate
	}
	refs, err := repo.ListCandidateSourceRefs(ctx, node.ID)
	if err != nil {
		return candidate
	}
	for _, ref := range refs {
		candidate.SourceTypes = append(candidate.SourceTypes, ref.SourceType)
		candidate.SourceIDs = append(candidate.SourceIDs, ref.SourceID)
	}
	return candidate
}
