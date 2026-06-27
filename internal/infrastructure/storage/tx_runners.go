package storage

import (
	"context"

	appnodes "proxygateway/internal/application/nodes"
	appprofiles "proxygateway/internal/application/profiles"
	appsubscriptions "proxygateway/internal/application/subscriptions"
	appuow "proxygateway/internal/application/uow"
)

type TxRunners struct {
	handle Handle
}

func NewTxRunners(handle Handle) TxRunners {
	return TxRunners{handle: handle}
}

func (r TxRunners) WithImportTx(ctx context.Context, fn func(appsubscriptions.ImportTx) error) error {
	return r.handle.WithTx(ctx, func(tx appuow.Tx) error {
		return fn(tx)
	})
}

func (r TxRunners) WithManualNodeImportTx(ctx context.Context, fn func(appsubscriptions.ManualNodeImportTx) error) error {
	return r.handle.WithTx(ctx, func(tx appuow.Tx) error {
		return fn(tx)
	})
}

func (r TxRunners) WithSubscriptionDeleteTx(ctx context.Context, fn func(appsubscriptions.DeleteTx) error) error {
	return r.handle.WithTx(ctx, func(tx appuow.Tx) error {
		return fn(tx)
	})
}

func (r TxRunners) WithManualUpdateTx(ctx context.Context, fn func(appnodes.ManualUpdateTx) error) error {
	return r.handle.WithTx(ctx, func(tx appuow.Tx) error {
		return fn(tx)
	})
}

func (r TxRunners) WithNodeUpsertTx(ctx context.Context, fn func(appnodes.UpsertTx) error) error {
	return r.handle.WithTx(ctx, func(tx appuow.Tx) error {
		return fn(tx)
	})
}

func (r TxRunners) WithNodeDeleteTx(ctx context.Context, fn func(appnodes.DeleteTx) error) error {
	return r.handle.WithTx(ctx, func(tx appuow.Tx) error {
		return fn(tx)
	})
}

func (r TxRunners) WithProfileDeleteTx(ctx context.Context, fn func(appprofiles.DeleteTx) error) error {
	return r.handle.WithTx(ctx, func(tx appuow.Tx) error {
		return fn(tx)
	})
}

func (r TxRunners) WithProfileConfigReleaseTx(ctx context.Context, fn func(appprofiles.ConfigReleaseTx) error) error {
	return r.handle.WithTx(ctx, func(tx appuow.Tx) error {
		return fn(tx)
	})
}
