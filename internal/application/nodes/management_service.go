package nodes

import "context"

type ManagementService struct {
	Repo   Repository
	Runner interface {
		UpsertTxRunner
		ManualUpdateTxRunner
		DeleteTxRunner
	}
	NewNodeID func() (string, error)
	Now       func() int64
}

func (s ManagementService) List(ctx context.Context, filter ListFilter) (map[string]any, error) {
	return NewReadService(s.Repo).List(ctx, filter)
}

func (s ManagementService) Detail(ctx context.Context, nodeID string) (map[string]any, error) {
	return NewReadService(s.Repo).Detail(ctx, nodeID)
}

func (s ManagementService) CreateManual(ctx context.Context, node OutboundNode) (string, error) {
	return CreateService{
		Runner:    s.Runner,
		NewNodeID: s.NewNodeID,
	}.Create(ctx, CreateCommand{
		Node:      node,
		Source:    SourceInput{ID: "manual", Name: "Manual", Type: "manual"},
		NowMillis: s.now(),
	})
}

func (s ManagementService) SetEnabled(ctx context.Context, nodeID string, enabled bool) (SetEnabledResult, error) {
	return SetEnabled(ctx, s.Repo, nodeID, enabled)
}

func (s ManagementService) UpdateManual(ctx context.Context, command ManualUpdateCommand) (ManualUpdateUseCaseResult, error) {
	command.NowMillis = s.now()
	return ManualUpdateService{
		Runner:    s.Runner,
		Nodes:     s.Repo,
		NewNodeID: s.NewNodeID,
	}.Update(ctx, command)
}

func (s ManagementService) DeleteManualSource(ctx context.Context, nodeID string) (DeleteResult, error) {
	return DeleteService{
		Runner: s.Runner,
	}.DeleteManualSource(ctx, nodeID)
}

func (s ManagementService) now() int64 {
	if s.Now == nil {
		return 0
	}
	return s.Now()
}
