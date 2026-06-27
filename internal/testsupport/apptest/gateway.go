package apptest

import (
	"testing"

	"proxygateway/internal/app"
)

func NewGateway(t testing.TB, opts ...app.Option) *app.Gateway {
	t.Helper()
	g, err := app.New(t.TempDir(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}
