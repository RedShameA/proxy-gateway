package nodes

import "context"

type UpsertTx interface {
	NodeUpsertRepository() UpsertRepository
}

type UpsertTxRunner interface {
	WithNodeUpsertTx(ctx context.Context, fn func(UpsertTx) error) error
}

type CreateCommand struct {
	Node      OutboundNode
	Source    SourceInput
	NowMillis int64
}

type CreateService struct {
	Runner    UpsertTxRunner
	NewNodeID func() (string, error)
}

func (s CreateService) Create(ctx context.Context, command CreateCommand) (string, error) {
	input, err := BuildUpsertInput(command.Node, command.Source, command.NowMillis)
	if err != nil {
		return "", err
	}
	nodeService := Service{NewNodeID: s.NewNodeID}
	var nodeID string
	err = s.Runner.WithNodeUpsertTx(ctx, func(tx UpsertTx) error {
		var upsertErr error
		nodeID, upsertErr = nodeService.Upsert(tx.NodeUpsertRepository(), input)
		return upsertErr
	})
	if err != nil {
		return "", err
	}
	return nodeID, nil
}
