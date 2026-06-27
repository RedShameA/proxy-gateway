package profiles

import "context"

type DeleteTx interface {
	ProfileDeleteRepository() DeleteRepository
}

type DeleteTxRunner interface {
	WithProfileDeleteTx(ctx context.Context, fn func(DeleteTx) error) error
}

type DeleteService struct {
	Runner DeleteTxRunner
}

func (s DeleteService) Delete(ctx context.Context, profileID string) (DeleteResult, error) {
	var result DeleteResult
	err := s.Runner.WithProfileDeleteTx(ctx, func(tx DeleteTx) error {
		var deleteErr error
		result, deleteErr = DeleteProfile(tx.ProfileDeleteRepository(), profileID)
		return deleteErr
	})
	if err != nil {
		return DeleteResult{}, err
	}
	return result, nil
}
