package evaluations

import (
	"context"
	"errors"
	"testing"

	appnodes "proxygateway/internal/application/nodes"
	domainprofile "proxygateway/internal/domain/profile"
)

func TestCandidateNodesAppliesCountrySourceAndNodeFilters(t *testing.T) {
	ctx := context.Background()
	repo := &fakeCandidateRepository{
		nodeIDs: mapOrder("node_manual_jp", "node_sub_us", "node_sub_jp", "node_missing"),
		countries: map[string]string{
			"node_manual_jp": "JP",
			"node_sub_us":    "US",
			"node_sub_jp":    "JP",
		},
		sourceRefs: map[string][]SourceRef{
			"node_manual_jp": {{SourceType: "manual", SourceID: "manual"}},
			"node_sub_us":    {{SourceType: "subscription", SourceID: "sub_1"}},
			"node_sub_jp":    {{SourceType: "subscription", SourceID: "sub_1"}},
		},
	}
	records := map[string]appnodes.Record{
		"node_manual_jp": {ID: "node_manual_jp", Name: "manual", Type: "http"},
		"node_sub_us":    {ID: "node_sub_us", Name: "us", Type: "http"},
		"node_sub_jp":    {ID: "node_sub_jp", Name: "jp", Type: "socks5"},
	}

	nodes, err := CandidateNodes(ctx, repo, fakeCandidateNodeLoader(records), domainprofile.CandidateFilter{
		EgressCountries: []string{"JP"},
		NodeSourceMode:  "specific_subscriptions",
		SourceIDs:       []string{"sub_1"},
		Protocols:       []string{"socks5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != "node_sub_jp" {
		t.Fatalf("candidate nodes = %#v", nodes)
	}
}

func TestCandidateNodesTreatsCountryLookupErrorAsUnknown(t *testing.T) {
	ctx := context.Background()
	repo := &fakeCandidateRepository{
		nodeIDs:    []string{"node_unknown"},
		countryErr: errors.New("lookup country"),
	}
	records := map[string]appnodes.Record{
		"node_unknown": {ID: "node_unknown", Name: "unknown", Type: "http"},
	}

	nodes, err := CandidateNodes(ctx, repo, fakeCandidateNodeLoader(records), domainprofile.CandidateFilter{
		EgressCountries: []string{"__unknown__"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != "node_unknown" {
		t.Fatalf("candidate nodes = %#v", nodes)
	}
}

type fakeCandidateRepository struct {
	nodeIDs    []string
	countries  map[string]string
	sourceRefs map[string][]SourceRef
	countryErr error
}

func (r *fakeCandidateRepository) ListCandidateNodeIDs(context.Context) ([]string, error) {
	return append([]string{}, r.nodeIDs...), nil
}

func (r *fakeCandidateRepository) CandidateEgressCountry(_ context.Context, nodeID string) (string, bool, error) {
	if r.countryErr != nil {
		return "", false, r.countryErr
	}
	country, ok := r.countries[nodeID]
	return country, ok, nil
}

func (r *fakeCandidateRepository) ListCandidateSourceRefs(_ context.Context, nodeID string) ([]SourceRef, error) {
	return append([]SourceRef{}, r.sourceRefs[nodeID]...), nil
}

func fakeCandidateNodeLoader(records map[string]appnodes.Record) CandidateNodeLoader {
	return func(_ context.Context, nodeID string) (appnodes.Record, bool, error) {
		record, ok := records[nodeID]
		return record, ok, nil
	}
}

func mapOrder(values ...string) []string {
	return append([]string{}, values...)
}
