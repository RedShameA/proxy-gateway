package app

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestGatewayServeDoesNotLogErrorWhenListenerClosed(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	g := &Gateway{logger: zap.New(core)}

	err := g.Serve(acceptErrorListener{err: fmt.Errorf("wrapped accept error: %w", net.ErrClosed)})

	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Serve error = %v, want net.ErrClosed", err)
	}
	for _, entry := range observed.FilterMessage("listener accept failed").All() {
		if entry.Level == zapcore.ErrorLevel {
			t.Fatalf("listener closed logged as error: %#v", entry)
		}
	}
}

func TestGatewayServeLogsErrorForUnexpectedAcceptFailure(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	g := &Gateway{logger: zap.New(core)}
	acceptErr := errors.New("accept failed")

	err := g.Serve(acceptErrorListener{err: acceptErr})

	if !errors.Is(err, acceptErr) {
		t.Fatalf("Serve error = %v, want %v", err, acceptErr)
	}
	errorLogs := 0
	for _, entry := range observed.FilterMessage("listener accept failed").All() {
		if entry.Level == zapcore.ErrorLevel {
			errorLogs++
		}
	}
	if errorLogs != 1 {
		t.Fatalf("listener accept failed error logs = %d, want 1", errorLogs)
	}
}

type acceptErrorListener struct {
	err error
}

func (l acceptErrorListener) Accept() (net.Conn, error) {
	return nil, l.err
}

func (acceptErrorListener) Close() error {
	return nil
}

func (acceptErrorListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}
