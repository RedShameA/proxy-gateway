package entrypoint

import (
	"bufio"
	"net"
	"net/http"
	"time"
)

const (
	DefaultSniffTimeout      = 5 * time.Second
	DefaultReadHeaderTimeout = 10 * time.Second
)

type Handler struct {
	HTTP              http.Handler
	SOCKS5            func(net.Conn)
	SniffTimeout      time.Duration
	ReadHeaderTimeout time.Duration
}

func (h Handler) ServeConn(conn net.Conn) {
	sniffTimeout := h.SniffTimeout
	if sniffTimeout == 0 {
		sniffTimeout = DefaultSniffTimeout
	}
	_ = conn.SetReadDeadline(time.Now().Add(sniffTimeout))
	reader := bufio.NewReader(conn)
	first, err := reader.Peek(1)
	if err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	buffered := &bufferedConn{Conn: conn, reader: reader}
	if first[0] == 0x05 {
		if h.SOCKS5 == nil {
			_ = buffered.Close()
			return
		}
		h.SOCKS5(buffered)
		return
	}
	server := &http.Server{
		Handler:           h.HTTP,
		ReadHeaderTimeout: h.readHeaderTimeout(),
	}
	_ = server.Serve(&singleConnListener{conn: buffered})
}

func (h Handler) readHeaderTimeout() time.Duration {
	if h.ReadHeaderTimeout != 0 {
		return h.ReadHeaderTimeout
	}
	return DefaultReadHeaderTimeout
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

type singleConnListener struct {
	conn net.Conn
	done bool
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.done {
		return nil, net.ErrClosed
	}
	l.done = true
	return l.conn, nil
}

func (l *singleConnListener) Close() error {
	if l.conn != nil && !l.done {
		return l.conn.Close()
	}
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}
