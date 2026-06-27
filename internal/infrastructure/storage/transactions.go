package storage

import (
	"context"
	"database/sql"
	"fmt"

	appnodes "proxygateway/internal/application/nodes"
	appprofiles "proxygateway/internal/application/profiles"
	appsubscriptions "proxygateway/internal/application/subscriptions"
	appuow "proxygateway/internal/application/uow"
	databaseinfra "proxygateway/internal/infrastructure/database"
	sqliteinfra "proxygateway/internal/infrastructure/sqlite"
)

type sqliteTx struct {
	tx *sql.Tx
}

func (h Handle) Close() error {
	if h.DB == nil {
		return nil
	}
	return h.DB.Close()
}

func (h Handle) WithTx(ctx context.Context, fn func(appuow.Tx) error) error {
	switch h.Dialect {
	case "", databaseinfra.DialectSQLite:
		return h.withSQLiteTx(ctx, fn)
	case databaseinfra.DialectPostgres:
		return fmt.Errorf("database dialect %q transactions are not implemented yet", h.Dialect)
	default:
		return fmt.Errorf("unsupported database dialect %q", h.Dialect)
	}
}

func (h Handle) withSQLiteTx(ctx context.Context, fn func(appuow.Tx) error) error {
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(sqliteTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (tx sqliteTx) NodeUpsertRepository() appnodes.UpsertRepository {
	return sqliteinfra.NewNodeUpsertRepositoryTx(tx.tx)
}

func (tx sqliteTx) NodeManualUpdateRepository() appnodes.ManualUpdateRepository {
	return sqliteinfra.NewNodeManualUpdateRepositoryTx(tx.tx)
}

func (tx sqliteTx) NodeDeleteRepository() appnodes.DeleteRepository {
	return sqliteinfra.NewNodeDeleteRepositoryTx(tx.tx)
}

func (tx sqliteTx) SubscriptionImportRepository() appsubscriptions.ImportRepository {
	return sqliteinfra.NewSubscriptionImportRepositoryTx(tx.tx)
}

func (tx sqliteTx) SubscriptionSourceRepository(nowMillis int64) appsubscriptions.SourceRepository {
	return sqliteinfra.NewSubscriptionSourceRepositoryTx(tx.tx, nowMillis)
}

func (tx sqliteTx) ProfileDeleteRepository() appprofiles.DeleteRepository {
	return sqliteinfra.NewProfileDeleteRepositoryTx(tx.tx)
}

func (tx sqliteTx) ProfileConfigRepository() appprofiles.ConfigUpdater {
	return sqliteinfra.NewProfileConfigRepositoryTx(tx.tx)
}

func (tx sqliteTx) ReleaseRetainedProfileNodesExcept(profileID string, keepNodeIDs []string) ([]string, error) {
	return sqliteinfra.ReleaseRetainedProfileNodesExceptTx(tx.tx, profileID, keepNodeIDs)
}
