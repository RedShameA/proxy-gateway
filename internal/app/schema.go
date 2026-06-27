package app

import (
	"context"

	storageinfra "proxygateway/internal/infrastructure/storage"
)

func (g *Gateway) migrate() error {
	return storageinfra.Migrate(context.Background(), g.store)
}
