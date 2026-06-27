package entrypoint

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHandlerDispatchesHTTPConnection(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	done := make(chan struct{})
	handler := Handler{
		HTTP: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() {
		defer close(done)
		handler.ServeConn(serverConn)
	}()
	if _, err := io.WriteString(clientConn, "GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := clientConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(clientConn)
	if err != nil && !strings.Contains(err.Error(), "timeout") {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "204 No Content") {
		t.Fatalf("HTTP response = %q, want 204 response", raw)
	}
	_ = clientConn.Close()
	<-done
}

func TestHandlerDispatchesSOCKS5ConnectionWithBufferedFirstByte(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	seen := make(chan byte, 1)
	handler := Handler{
		SOCKS5: func(conn net.Conn) {
			defer conn.Close()
			buf := []byte{0}
			if _, err := io.ReadFull(conn, buf); err != nil {
				t.Errorf("read SOCKS5 first byte: %v", err)
				return
			}
			seen <- buf[0]
		},
	}
	go handler.ServeConn(serverConn)
	if _, err := clientConn.Write([]byte{0x05}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-seen:
		if got != 0x05 {
			t.Fatalf("SOCKS5 first byte = %#x, want 0x05", got)
		}
	case <-time.After(time.Second):
		t.Fatal("SOCKS5 handler was not called")
	}
}
