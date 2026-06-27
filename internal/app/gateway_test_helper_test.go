package app

import "testing"

func NewForTest(t testing.TB, opts ...Option) *Gateway {
	t.Helper()
	g, err := New(t.TempDir(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}
