package subscriptions

import "context"

type DeleteTx interface {
	SubscriptionSourceRepository(nowMillis int64) SourceRepository
}

type DeleteTxRunner interface {
	WithSubscriptionDeleteTx(ctx context.Context, fn func(DeleteTx) error) error
}

type DeleteService struct {
	Runner DeleteTxRunner
	Now    func() int64
}

func (s DeleteService) Delete(ctx context.Context, subscriptionID string) (DeleteResult, error) {
	now := int64(0)
	if s.Now != nil {
		now = s.Now()
	}
	var result DeleteResult
	err := s.Runner.WithSubscriptionDeleteTx(ctx, func(tx DeleteTx) error {
		var deleteErr error
		result, deleteErr = DeleteSubscriptionSource(tx.SubscriptionSourceRepository(now), subscriptionID)
		return deleteErr
	})
	if err != nil {
		return DeleteResult{}, err
	}
	return result, nil
}
