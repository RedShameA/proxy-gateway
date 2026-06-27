package nodes

import "context"

type DeleteTx interface {
	NodeDeleteRepository() DeleteRepository
}

type DeleteTxRunner interface {
	WithNodeDeleteTx(ctx context.Context, fn func(DeleteTx) error) error
}

type DeleteService struct {
	Runner DeleteTxRunner
}

func (s DeleteService) DeleteManualSource(ctx context.Context, nodeID string) (DeleteResult, error) {
	var result DeleteResult
	err := s.Runner.WithNodeDeleteTx(ctx, func(tx DeleteTx) error {
		var deleteErr error
		result, deleteErr = DeleteManualSource(tx.NodeDeleteRepository(), nodeID)
		return deleteErr
	})
	if err != nil {
		return DeleteResult{}, err
	}
	return result, nil
}
