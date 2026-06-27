package profiles

import "context"

type ManagementService struct {
	Config      ConfigService
	Credentials CredentialService
	Summary     SummaryService
	Detail      DetailService
	Delete      DeleteService
}

func (s ManagementService) Create(ctx context.Context, req PatchRequest) (ConfigMutationResult, error) {
	return s.Config.Create(ctx, req)
}

func (s ManagementService) Update(ctx context.Context, profileID string, req PatchRequest) (ConfigMutationResult, error) {
	return s.Config.Update(ctx, profileID, req)
}

func (s ManagementService) DeleteProfile(ctx context.Context, profileID string) (DeleteResult, error) {
	return s.Delete.Delete(ctx, profileID)
}

func (s ManagementService) List(ctx context.Context, filter ListConfigFilter) (any, error) {
	return s.Summary.List(ctx, filter)
}

func (s ManagementService) ListSummaries(ctx context.Context, filter ListConfigFilter) ([]Summary, error) {
	return s.Summary.ListSummaries(ctx, filter)
}

func (s ManagementService) BuildSummary(ctx context.Context, cfg ConfigRecord) Summary {
	return s.Summary.Build(ctx, cfg)
}

func (s ManagementService) CreateCredential(ctx context.Context, command CreateCredentialCommand) (CreatedCredential, error) {
	return s.Credentials.Create(ctx, command)
}

func (s ManagementService) ListCredentials(ctx context.Context, profileID, endpoint string) (CredentialList, error) {
	return s.Credentials.List(ctx, profileID, endpoint)
}

func (s ManagementService) SetCredentialEnabled(ctx context.Context, profileID, credentialID string, enabled bool) (UpdateCredentialResult, error) {
	return s.Credentials.SetEnabled(ctx, profileID, credentialID, enabled)
}

func (s ManagementService) DeleteCredential(ctx context.Context, profileID, credentialID string) (DeleteCredentialResult, error) {
	return s.Credentials.Delete(ctx, profileID, credentialID)
}

func (s ManagementService) LoadDetail(ctx context.Context, profileID, endpoint string) (Detail, error) {
	return s.Detail.Load(ctx, profileID, endpoint)
}
