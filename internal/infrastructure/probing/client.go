package probing

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	appproxy "proxygateway/internal/application/proxy"
)

const responseBodyLimit = 64 * 1024

var errNilDialConn = errors.New("node protocol engine returned nil connection")

type OutboundGETRequest struct {
	TargetHost string
	Request    *http.Request
	HTTPS      bool
	ServerName string
}

type HTTPResult struct {
	DurationMS int64
	HTTPStatus int
	Body       []byte
}

type Client struct {
	Engine appproxy.NodeProtocolEngine
}

func (c Client) FetchThroughNode(node appproxy.Node, rawURL string, timeouts appproxy.DialTimeouts) (HTTPResult, error) {
	outbound, err := BuildOutboundGETRequest(rawURL)
	if err != nil {
		return HTTPResult{}, err
	}
	if c.Engine == nil {
		return HTTPResult{}, errors.New("node protocol engine is nil")
	}
	start := time.Now()
	conn, err := c.Engine.DialNode(node, outbound.TargetHost, timeouts)
	if err != nil {
		return HTTPResult{}, err
	}
	if isNilConn(conn) {
		return HTTPResult{}, errNilDialConn
	}
	return fetchWithConn(conn, outbound, timeouts, start)
}

func (c Client) FetchThroughChain(frontNode, exitNode appproxy.Node, rawURL string, timeouts appproxy.DialTimeouts) (HTTPResult, error) {
	outbound, err := BuildOutboundGETRequest(rawURL)
	if err != nil {
		return HTTPResult{}, err
	}
	if c.Engine == nil {
		return HTTPResult{}, errors.New("node protocol engine is nil")
	}
	start := time.Now()
	conn, err := c.Engine.DialChain(frontNode, exitNode, outbound.TargetHost, timeouts)
	if err != nil {
		return HTTPResult{}, err
	}
	if isNilConn(conn) {
		return HTTPResult{}, errNilDialConn
	}
	return fetchWithConn(conn, outbound, timeouts, start)
}

func (c Client) ProbeChainLink(frontNode, exitNode appproxy.Node, rawURL string, timeouts appproxy.DialTimeouts) (int64, error) {
	outbound, err := BuildOutboundGETRequest(rawURL)
	if err != nil {
		return 0, err
	}
	if c.Engine == nil {
		return 0, errors.New("node protocol engine is nil")
	}
	start := time.Now()
	conn, err := c.Engine.DialChain(frontNode, exitNode, outbound.TargetHost, timeouts)
	if err != nil {
		return 0, err
	}
	if isNilConn(conn) {
		return 0, errNilDialConn
	}
	_ = conn.Close()
	return time.Since(start).Milliseconds(), nil
}

func fetchWithConn(conn net.Conn, outbound OutboundGETRequest, timeouts appproxy.DialTimeouts, start time.Time) (HTTPResult, error) {
	if isNilConn(conn) {
		return HTTPResult{}, errNilDialConn
	}
	defer conn.Close()
	if !timeouts.Deadline.IsZero() {
		_ = conn.SetDeadline(timeouts.Deadline)
		defer func(conn net.Conn) { _ = conn.SetDeadline(time.Time{}) }(conn)
	}
	conn, err := WrapOutboundGETConn(conn, outbound)
	if err != nil {
		return HTTPResult{}, err
	}
	if isNilConn(conn) {
		return HTTPResult{}, errNilDialConn
	}
	if err := outbound.Request.Write(conn); err != nil {
		return HTTPResult{}, err
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), outbound.Request)
	if err != nil {
		return HTTPResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, responseBodyLimit))
	if err != nil {
		return HTTPResult{}, err
	}
	return HTTPResult{
		DurationMS: time.Since(start).Milliseconds(),
		HTTPStatus: resp.StatusCode,
		Body:       body,
	}, nil
}

func isNilConn(conn net.Conn) bool {
	if conn == nil {
		return true
	}
	value := reflect.ValueOf(conn)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func BuildOutboundGET(rawURL string) (string, *http.Request, error) {
	outbound, err := BuildOutboundGETRequest(rawURL)
	if err != nil {
		return "", nil, err
	}
	return outbound.TargetHost, outbound.Request, nil
}

func BuildOutboundGETRequest(rawURL string) (OutboundGETRequest, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return OutboundGETRequest{}, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return OutboundGETRequest{}, errors.New("Test URL 仅支持 http:// 或 https://")
	}
	if strings.TrimSpace(u.Host) == "" {
		return OutboundGETRequest{}, errors.New("Test URL 必须包含主机名")
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	targetHost := net.JoinHostPort(u.Hostname(), port)
	req, err := http.NewRequest(http.MethodGet, u.RequestURI(), nil)
	if err != nil {
		return OutboundGETRequest{}, err
	}
	req.Host = u.Host
	return OutboundGETRequest{
		TargetHost: targetHost,
		Request:    req,
		HTTPS:      u.Scheme == "https",
		ServerName: u.Hostname(),
	}, nil
}

func WrapOutboundGETConn(conn net.Conn, outbound OutboundGETRequest) (net.Conn, error) {
	if !outbound.HTTPS {
		return conn, nil
	}
	cfg := &tls.Config{ServerName: outbound.ServerName}
	if ip := net.ParseIP(outbound.ServerName); ip != nil && ip.IsLoopback() {
		cfg.InsecureSkipVerify = true
	}
	tlsConn := tls.Client(conn, cfg)
	if err := tlsConn.Handshake(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return tlsConn, nil
}
