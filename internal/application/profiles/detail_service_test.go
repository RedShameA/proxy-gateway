package profiles

import (
	"context"
	"errors"
	"reflect"
	"testing"

	domainprofile "proxygateway/internal/domain/profile"
)

func TestDetailServiceBuildsProfileDetailReadModel(t *testing.T) {
	configs := newFakeConfigRepository()
	cfg := DefaultConfig("profile_1")
	cfg.Name = "Work"
	cfg.ProfileIdentifier = "work"
	cfg.Type = "fastest"
	cfg.NodeSourceMode = "specific_subscriptions"
	cfg.SourceIDs = []string{"sub_1"}
	cfg.Protocols = []string{"http"}
	cfg.EgressCountries = []string{"JP"}
	cfg.LastEvaluationDetailsJSON = `{"candidate_count":2}`
	configs.records[cfg.ID] = cfg
	credentials := &detailServiceCredentialRepository{
		records: []CredentialRecord{
			{ID: "cred_1", ProfileID: cfg.ID, Remark: "client", Password: "secret", Enabled: true, CreatedAt: 100},
		},
		counts: CredentialCounts{Total: 1, Enabled: 1},
	}
	service := NewDetailService(DetailServiceDeps{
		Configs:     configs,
		Credentials: credentials,
		CandidateNodeIDs: func(filter domainprofile.CandidateFilter) ([]string, error) {
			if !reflect.DeepEqual(filter.SourceIDs, []string{"sub_1"}) {
				t.Fatalf("candidate filter = %#v", filter)
			}
			return []string{"node_1", "node_2"}, nil
		},
		NodeUsable: func(nodeID string) bool {
			return nodeID == "node_1"
		},
		UnknownCountryCandidateCount: func(domainprofile.CandidateFilter) int {
			return 1
		},
		CurrentPath: func(record ConfigRecord) any {
			return map[string]any{"path_type": "single", "profile_id": record.ID}
		},
		RecentEvents: func(_ context.Context, profileID string, limit int) ([]map[string]any, error) {
			if profileID != cfg.ID || limit != 10 {
				t.Fatalf("recent events args = %q %d", profileID, limit)
			}
			return []map[string]any{{"id": "run_1"}}, nil
		},
	})

	detail, err := service.Load(context.Background(), cfg.ID, "127.0.0.1:28080")
	if err != nil {
		t.Fatal(err)
	}

	if detail.ID != cfg.ID || detail.ProfileIdentifier != "work" || detail.CandidateFilter.SourceMode != "selected_sources" {
		t.Fatalf("detail identity/filter = %#v", detail)
	}
	if detail.ProxyCredentials[0].HTTPProxyURL != "http://work:secret@127.0.0.1:28080" {
		t.Fatalf("credential URL = %q", detail.ProxyCredentials[0].HTTPProxyURL)
	}
	if detail.CandidateStats.Total != 2 || detail.CandidateStats.Usable != 1 || detail.CandidateStats.UnknownEgressCountry != 1 {
		t.Fatalf("candidate stats = %#v", detail.CandidateStats)
	}
	if detail.LastEvaluationDetails["candidate_count"] != float64(2) {
		t.Fatalf("last evaluation details = %#v", detail.LastEvaluationDetails)
	}
	if len(detail.RecentEvents) != 1 || detail.RecentEvents[0]["id"] != "run_1" {
		t.Fatalf("recent events = %#v", detail.RecentEvents)
	}
}

func TestDetailServiceReturnsProfileNotFound(t *testing.T) {
	service := NewDetailService(DetailServiceDeps{Configs: newFakeConfigRepository()})

	_, err := service.Load(context.Background(), "missing", "127.0.0.1:28080")
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Load error = %v, want ErrProfileNotFound", err)
	}
}

type detailServiceCredentialRepository struct {
	records []CredentialRecord
	counts  CredentialCounts
}

func (r *detailServiceCredentialRepository) ProfileExists(context.Context, string) (bool, error) {
	return true, nil
}

func (r *detailServiceCredentialRepository) LoadProfileIdentifier(_ context.Context, profileID string) (string, bool, error) {
	return profileID, true, nil
}

func (r *detailServiceCredentialRepository) PasswordExists(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *detailServiceCredentialRepository) CreateCredential(context.Context, CredentialCreateRecord) error {
	return nil
}

func (r *detailServiceCredentialRepository) ListCredentials(context.Context, string) ([]CredentialRecord, error) {
	return r.records, nil
}

func (r *detailServiceCredentialRepository) SetCredentialEnabled(context.Context, string, string, bool) (bool, error) {
	return false, nil
}

func (r *detailServiceCredentialRepository) DeleteCredential(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *detailServiceCredentialRepository) CountCredentials(context.Context, string) (CredentialCounts, error) {
	return r.counts, nil
}
