package main

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	storageinfra "proxygateway/internal/infrastructure/storage"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRunWiresStorageConfigFromEnv(t *testing.T) {
	t.Parallel()

	var captured storageinfra.Config
	deps := testRunDeps(t)
	deps.lookupEnv = func(key string) (string, bool) {
		switch key {
		case storageinfra.EnvDBDriver:
			return "postgresql", true
		case storageinfra.EnvDBDSN:
			return "postgres://proxy:secret@example.local/proxygateway", true
		default:
			return "", false
		}
	}
	deps.newGateway = func(dataDir string, logger *zap.Logger, config storageinfra.Config) (gatewayRunner, error) {
		captured = config
		return fakeGatewayRunner{}, nil
	}

	if code := runWithDeps(deps); code != 1 {
		t.Fatalf("exit code = %d, want 1 after fake serve stops", code)
	}
	if captured.Driver != storageinfra.DriverPostgres {
		t.Fatalf("driver = %q, want %q", captured.Driver, storageinfra.DriverPostgres)
	}
	if captured.DSN != "postgres://proxy:secret@example.local/proxygateway" {
		t.Fatalf("DSN was not passed to app.New")
	}
}

func TestRunLogsNormalizedPostgresDriverOnGatewayOpenFailure(t *testing.T) {
	t.Parallel()

	core, logs := observer.New(zapcore.DebugLevel)
	deps := testRunDeps(t)
	deps.newLogger = func() (*zap.Logger, error) {
		return zap.New(core), nil
	}
	deps.lookupEnv = func(key string) (string, bool) {
		switch key {
		case storageinfra.EnvDBDriver:
			return "postgresql", true
		case storageinfra.EnvDBDSN:
			return "postgres://proxy:secret@example.local/proxygateway", true
		default:
			return "", false
		}
	}
	deps.newGateway = func(string, *zap.Logger, storageinfra.Config) (gatewayRunner, error) {
		return nil, errors.New("open failed")
	}

	if code := runWithDeps(deps); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	entry := logs.FilterMessage("open gateway failed").TakeAll()
	if len(entry) != 1 {
		t.Fatalf("open gateway failed logs = %d, want 1", len(entry))
	}
	fields := entry[0].ContextMap()
	if fields["db_driver"] != storageinfra.DriverPostgres {
		t.Fatalf("db_driver log field = %#v, want %q", fields["db_driver"], storageinfra.DriverPostgres)
	}
}

func TestRunUsesDefaultSQLiteStorageConfig(t *testing.T) {
	t.Parallel()

	var captured storageinfra.Config
	deps := testRunDeps(t)
	deps.newGateway = func(dataDir string, logger *zap.Logger, config storageinfra.Config) (gatewayRunner, error) {
		captured = config
		return fakeGatewayRunner{}, nil
	}

	if code := runWithDeps(deps); code != 1 {
		t.Fatalf("exit code = %d, want 1 after fake serve stops", code)
	}
	if captured.Driver != storageinfra.DriverSQLite {
		t.Fatalf("driver = %q, want %q", captured.Driver, storageinfra.DriverSQLite)
	}
	if captured.DataDir == "" {
		t.Fatal("DataDir was not passed to app.New")
	}
}

func TestRunRejectsDSNWithoutDriverBeforeOpeningGateway(t *testing.T) {
	t.Parallel()

	deps := testRunDeps(t)
	deps.lookupEnv = func(key string) (string, bool) {
		if key == storageinfra.EnvDBDSN {
			return "postgres://proxy:secret@example.local/proxygateway", true
		}
		return "", false
	}
	deps.newGateway = func(string, *zap.Logger, storageinfra.Config) (gatewayRunner, error) {
		t.Fatal("newGateway should not be called when database config is invalid")
		return nil, nil
	}

	if code := runWithDeps(deps); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestRunRejectsInvalidDriverBeforeOpeningGateway(t *testing.T) {
	t.Parallel()

	deps := testRunDeps(t)
	deps.lookupEnv = func(key string) (string, bool) {
		if key == storageinfra.EnvDBDriver {
			return "mysql", true
		}
		return "", false
	}
	deps.newGateway = func(string, *zap.Logger, storageinfra.Config) (gatewayRunner, error) {
		t.Fatal("newGateway should not be called when database driver is invalid")
		return nil, nil
	}

	if code := runWithDeps(deps); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func testRunDeps(t *testing.T) runDeps {
	t.Helper()
	return runDeps{
		dataDir:    t.TempDir(),
		listenAddr: "127.0.0.1:0",
		newLogger: func() (*zap.Logger, error) {
			return zap.NewNop(), nil
		},
		lookupEnv: func(string) (string, bool) {
			return "", false
		},
		newGateway: func(string, *zap.Logger, storageinfra.Config) (gatewayRunner, error) {
			return fakeGatewayRunner{}, nil
		},
		listen: func(network, addr string) (net.Listener, error) {
			return net.Listen(network, addr)
		},
		notifyContext: func(parent context.Context) (context.Context, context.CancelFunc) {
			return parent, func() {}
		},
	}
}

func TestRunReturnsZeroOnGracefulShutdown(t *testing.T) {
	shutdownCtx, cancel := context.WithCancel(context.Background())
	serveStarted := make(chan struct{})
	var closed atomic.Bool

	deps := testRunDeps(t)
	deps.notifyContext = func(context.Context) (context.Context, context.CancelFunc) {
		return shutdownCtx, func() {}
	}
	deps.newGateway = func(string, *zap.Logger, storageinfra.Config) (gatewayRunner, error) {
		return fakeGatewayRunner{
			close: func() error {
				closed.Store(true)
				return nil
			},
			serve: func(ln net.Listener) error {
				close(serveStarted)
				_, err := ln.Accept()
				return err
			},
		}, nil
	}

	done := make(chan int, 1)
	go func() {
		done <- runWithDeps(deps)
	}()
	<-serveStarted
	cancel()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runWithDeps did not return after shutdown")
	}
	if !closed.Load() {
		t.Fatal("gateway Close was not called")
	}
}

func TestRunDoesNotTreatStopCleanupAsGracefulShutdown(t *testing.T) {
	deps := testRunDeps(t)
	deps.notifyContext = func(parent context.Context) (context.Context, context.CancelFunc) {
		return context.WithCancel(parent)
	}

	if code := runWithDeps(deps); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestRunDoesNotTreatNonListenerClosedServeErrorAsGracefulShutdown(t *testing.T) {
	shutdownCtx, cancel := context.WithCancel(context.Background())
	cancel()

	deps := testRunDeps(t)
	deps.notifyContext = func(context.Context) (context.Context, context.CancelFunc) {
		return shutdownCtx, func() {}
	}

	if code := runWithDeps(deps); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestRunReturnsZeroWhenServeReturnsNil(t *testing.T) {
	var closed atomic.Bool
	deps := testRunDeps(t)
	deps.newGateway = func(string, *zap.Logger, storageinfra.Config) (gatewayRunner, error) {
		return fakeGatewayRunner{
			close: func() error {
				closed.Store(true)
				return nil
			},
			serve: func(ln net.Listener) error {
				_ = ln.Close()
				return nil
			},
		}, nil
	}

	if code := runWithDeps(deps); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !closed.Load() {
		t.Fatal("gateway Close was not called")
	}
}

type fakeGatewayRunner struct {
	close func() error
	serve func(net.Listener) error
}

func (f fakeGatewayRunner) Close() error {
	if f.close != nil {
		return f.close()
	}
	return nil
}

func (f fakeGatewayRunner) Serve(ln net.Listener) error {
	if f.serve != nil {
		return f.serve(ln)
	}
	_ = ln.Close()
	return errors.New("fake serve stopped")
}
