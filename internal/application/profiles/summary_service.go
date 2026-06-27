package profiles

import "context"

type SummaryServiceDeps struct {
	Configs     ConfigRepository
	Credentials CredentialRepository
	CurrentPath CurrentPathBuilder
}

type SummaryService struct {
	deps SummaryServiceDeps
}

func NewSummaryService(deps SummaryServiceDeps) SummaryService {
	return SummaryService{deps: deps}
}

func (s SummaryService) Build(ctx context.Context, cfg ConfigRecord) Summary {
	return BuildSummary(SummaryInputFromConfig(cfg, s.currentPath(cfg), s.credentialCounts(ctx, cfg.ID)))
}

func (s SummaryService) List(ctx context.Context, filter ListConfigFilter) (SummaryList, error) {
	list, err := s.deps.Configs.ListConfigIDs(ctx, filter)
	if err != nil {
		return SummaryList{}, err
	}
	summaries := s.loadSummaries(ctx, list.IDs)
	return BuildSummaryList(summaries, list.Total), nil
}

func (s SummaryService) ListSummaries(ctx context.Context, filter ListConfigFilter) ([]Summary, error) {
	list, err := s.deps.Configs.ListConfigIDs(ctx, filter)
	if err != nil {
		return []Summary{}, err
	}
	return s.loadSummaries(ctx, list.IDs), nil
}

func (s SummaryService) loadSummaries(ctx context.Context, ids []string) []Summary {
	summaries := make([]Summary, 0, len(ids))
	for _, profileID := range ids {
		cfg, found, err := s.deps.Configs.LoadConfig(ctx, profileID)
		if err != nil || !found {
			continue
		}
		cfg.ApplyDefaults()
		summaries = append(summaries, s.Build(ctx, cfg))
	}
	if summaries == nil {
		return []Summary{}
	}
	return summaries
}

func (s SummaryService) credentialCounts(ctx context.Context, profileID string) CredentialCounts {
	if s.deps.Credentials == nil {
		return CredentialCounts{}
	}
	counts, _ := s.deps.Credentials.CountCredentials(ctx, profileID)
	return counts
}

func (s SummaryService) currentPath(cfg ConfigRecord) any {
	if s.deps.CurrentPath == nil {
		return nil
	}
	return s.deps.CurrentPath(cfg)
}
