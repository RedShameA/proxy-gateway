package nodes

import "context"

type ManualUpdateTx interface {
	NodeManualUpdateRepository() ManualUpdateRepository
}

type ManualUpdateTxRunner interface {
	WithManualUpdateTx(ctx context.Context, fn func(ManualUpdateTx) error) error
}

type ManualUpdateCommand struct {
	NodeID    string
	Node      OutboundNode
	Enabled   *bool
	NowMillis int64
}

type ManualUpdateUseCaseResult struct {
	NodeID                  string
	Split                   bool
	InvalidatedFingerprints []string
}

type ManualUpdateService struct {
	Runner    ManualUpdateTxRunner
	Nodes     Repository
	NewNodeID func() (string, error)
}

func (s ManualUpdateService) Update(ctx context.Context, command ManualUpdateCommand) (ManualUpdateUseCaseResult, error) {
	input, err := BuildManualUpdateInput(command.NodeID, command.Node, command.Enabled, command.NowMillis)
	if err != nil {
		return ManualUpdateUseCaseResult{}, err
	}
	oldNode, found, err := s.Nodes.Load(ctx, command.NodeID)
	if err != nil {
		return ManualUpdateUseCaseResult{}, err
	}
	if !found {
		return ManualUpdateUseCaseResult{}, ErrNodeNotFound
	}
	oldFingerprint := RuntimeFingerprint(oldNode)
	var result ManualUpdateResult
	err = s.Runner.WithManualUpdateTx(ctx, func(tx ManualUpdateTx) error {
		var updateErr error
		result, updateErr = Service{NewNodeID: s.NewNodeID}.UpdateManual(tx.NodeManualUpdateRepository(), input)
		return updateErr
	})
	if err != nil {
		return ManualUpdateUseCaseResult{}, err
	}
	out := ManualUpdateUseCaseResult{
		NodeID: result.NodeID,
		Split:  result.Split,
	}
	if !result.Split && oldFingerprint != "" && oldFingerprint != input.Fingerprint {
		out.InvalidatedFingerprints = append(out.InvalidatedFingerprints, oldFingerprint)
	}
	return out, nil
}
