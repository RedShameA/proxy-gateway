package profiles

import (
	"context"
	"errors"
	"sort"
	"testing"
)

func TestConfigServiceCreatesConfigAndReportsSideEffects(t *testing.T) {
	repo := newFakeConfigRepository()
	name := "Work"
	countries := []string{"JP"}
	service := NewConfigService(ConfigServiceDeps{
		Repository: repo,
		NewID: func() (string, error) {
			return "profile_1", nil
		},
		Now: func() int64 {
			return 1234
		},
		Validation: configMutationTestDeps(),
	})

	result, err := service.Create(context.Background(), PatchRequest{
		Name:            &name,
		EgressCountries: &countries,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.created.ID != "profile_1" || repo.createdAt != 1234 {
		t.Fatalf("created config = %#v at %d", repo.created, repo.createdAt)
	}
	if !result.EnqueueEvaluation || !result.EnqueueUnknownCountryObservation {
		t.Fatalf("side effects = %#v", result)
	}
	if result.Config.Name != "Work" || result.Config.EgressCountries[0] != "JP" {
		t.Fatalf("result config = %#v", result.Config)
	}
}

func TestConfigServiceUpdateUsesReleaseTransactionWhenPlanRequiresIt(t *testing.T) {
	repo := newFakeConfigRepository()
	original := DefaultConfig("profile_1")
	original.Name = "Work"
	original.EgressCountries = []string{"US"}
	original.CurrentNodeID = "node_current"
	original.State = "ready"
	original.NodeStickyEnabled = true
	original.ConfigVersion = 7
	repo.records[original.ID] = original

	var releaseProfileID string
	var releasedOptions ConfigUpdateOptions
	var releasedConfig ConfigRecord
	countries := []string{"JP"}
	service := NewConfigService(ConfigServiceDeps{
		Repository: repo,
		Validation: configMutationTestDeps(),
		UpdateWithRelease: func(_ context.Context, profileID string, record ConfigRecord, options ConfigUpdateOptions) ([]string, error) {
			releaseProfileID = profileID
			releasedConfig = record
			releasedOptions = options
			return []string{"fp_deleted"}, nil
		},
	})

	result, err := service.Update(context.Background(), "profile_1", PatchRequest{EgressCountries: &countries})
	if err != nil {
		t.Fatal(err)
	}
	if repo.updated.ID != "" {
		t.Fatalf("direct repository update should not be used when release is required: %#v", repo.updated)
	}
	if releaseProfileID != "profile_1" || releasedConfig.ConfigVersion != 8 {
		t.Fatalf("release update = profile %q config %#v", releaseProfileID, releasedConfig)
	}
	if !releasedOptions.EvaluationChanged || !releasedOptions.ResetCurrentPath {
		t.Fatalf("release options = %#v", releasedOptions)
	}
	if len(result.DeletedFingerprints) != 1 || result.DeletedFingerprints[0] != "fp_deleted" {
		t.Fatalf("deleted fingerprints = %#v", result.DeletedFingerprints)
	}
	if !result.EnqueueEvaluation || !result.EnqueueUnknownCountryObservation {
		t.Fatalf("side effects = %#v", result)
	}
}

func TestConfigServiceUpdateReturnsNotFound(t *testing.T) {
	service := NewConfigService(ConfigServiceDeps{
		Repository: newFakeConfigRepository(),
		Validation: configMutationTestDeps(),
	})

	_, err := service.Update(context.Background(), "profile_missing", PatchRequest{})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("Update error = %v, want ErrProfileNotFound", err)
	}
}

type fakeConfigRepository struct {
	records   map[string]ConfigRecord
	created   ConfigRecord
	createdAt int64
	updated   ConfigRecord
	options   ConfigUpdateOptions
}

func newFakeConfigRepository() *fakeConfigRepository {
	return &fakeConfigRepository{records: map[string]ConfigRecord{}}
}

func (r *fakeConfigRepository) ListConfigIDs(_ context.Context, filter ListConfigFilter) (ListConfigResult, error) {
	ids := make([]string, 0, len(r.records))
	for id := range r.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	total := len(ids)
	if filter.Offset > 0 && filter.Offset < len(ids) {
		ids = ids[filter.Offset:]
	} else if filter.Offset >= len(ids) {
		ids = []string{}
	}
	if filter.Limit > 0 && filter.Limit < len(ids) {
		ids = ids[:filter.Limit]
	}
	return ListConfigResult{IDs: ids, Total: total}, nil
}

func (r *fakeConfigRepository) LoadConfig(_ context.Context, profileID string) (ConfigRecord, bool, error) {
	record, ok := r.records[profileID]
	return record, ok, nil
}

func (r *fakeConfigRepository) CreateConfig(_ context.Context, record ConfigRecord, createdAt int64) error {
	r.created = record
	r.createdAt = createdAt
	r.records[record.ID] = record
	return nil
}

func (r *fakeConfigRepository) UpdateConfig(_ context.Context, record ConfigRecord, options ConfigUpdateOptions) error {
	r.updated = record
	r.options = options
	r.records[record.ID] = record
	return nil
}

func (r *fakeConfigRepository) ProfileIdentifierExists(_ context.Context, identifier, excludeProfileID string) (bool, error) {
	for _, record := range r.records {
		if record.ProfileIdentifier == identifier && record.ID != excludeProfileID {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeConfigRepository) Exists(_ context.Context, profileID string) (bool, error) {
	_, ok := r.records[profileID]
	return ok, nil
}
