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
	DurationMS     int64
	DialDurationMS int64
	HTTPDurationMS int64
	HTTPStatus     int
	Body           []byte
	DialMetrics    appproxy.DialMetrics
}

type DialProbeResult struct {
	DurationMS     int64
	DialDurationMS int64
	DialMetrics    appproxy.DialMetrics
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
	dialStart := time.Now()
	dialResult, err := c.Engine.DialNode(node, outbound.TargetHost, timeouts)
	dialDuration := elapsedMillis(dialStart)
	if err != nil {
		return httpResultWithElapsed(start, dialDuration, dialResult.Metrics), err
	}
	if isNilConn(dialResult.Conn) {
		return httpResultWithElapsed(start, dialDuration, dialResult.Metrics), errNilDialConn
	}
	return fetchWithConn(dialResult.Conn, outbound, timeouts, start, dialDuration, dialResult.Metrics)
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
	dialStart := time.Now()
	dialResult, err := c.Engine.DialChain(frontNode, exitNode, outbound.TargetHost, timeouts)
	dialDuration := elapsedMillis(dialStart)
	if err != nil {
		return httpResultWithElapsed(start, dialDuration, dialResult.Metrics), err
	}
	if isNilConn(dialResult.Conn) {
		return httpResultWithElapsed(start, dialDuration, dialResult.Metrics), errNilDialConn
	}
	return fetchWithConn(dialResult.Conn, outbound, timeouts, start, dialDuration, dialResult.Metrics)
}

func (c Client) ProbeChainLink(frontNode, exitNode appproxy.Node, rawURL string, timeouts appproxy.DialTimeouts) (DialProbeResult, error) {
	outbound, err := BuildOutboundGETRequest(rawURL)
	if err != nil {
		return DialProbeResult{}, err
	}
	if c.Engine == nil {
		return DialProbeResult{}, errors.New("node protocol engine is nil")
	}
	start := time.Now()
	dialStart := time.Now()
	dialResult, err := c.Engine.DialChain(frontNode, exitNode, outbound.TargetHost, timeouts)
	dialDuration := elapsedMillis(dialStart)
	result := DialProbeResult{
		DurationMS:     elapsedMillis(start),
		DialDurationMS: dialDuration,
		DialMetrics:    dialResult.Metrics,
	}
	if err != nil {
		result.DurationMS = elapsedMillis(start)
		return result, err
	}
	if isNilConn(dialResult.Conn) {
		result.DurationMS = elapsedMillis(start)
		return result, errNilDialConn
	}
	_ = dialResult.Conn.Close()
	result.DurationMS = elapsedMillis(start)
	return result, nil
}

func fetchWithConn(conn net.Conn, outbound OutboundGETRequest, timeouts appproxy.DialTimeouts, start time.Time, dialDuration int64, dialMetrics appproxy.DialMetrics) (HTTPResult, error) {
	if isNilConn(conn) {
		return HTTPResult{}, errNilDialConn
	}
	defer conn.Close()
	httpStart := time.Now()
	if !timeouts.Deadline.IsZero() {
		_ = conn.SetDeadline(timeouts.Deadline)
		defer func(conn net.Conn) { _ = conn.SetDeadline(time.Time{}) }(conn)
	}
	conn, err := WrapOutboundGETConn(conn, outbound)
	if err != nil {
		return httpResultWithElapsedAndHTTP(start, dialDuration, dialMetrics, httpStart), err
	}
	if isNilConn(conn) {
		return httpResultWithElapsedAndHTTP(start, dialDuration, dialMetrics, httpStart), errNilDialConn
	}
	if err := outbound.Request.Write(conn); err != nil {
		return httpResultWithElapsedAndHTTP(start, dialDuration, dialMetrics, httpStart), err
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), outbound.Request)
	if err != nil {
		return httpResultWithElapsedAndHTTP(start, dialDuration, dialMetrics, httpStart), err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, responseBodyLimit))
	if err != nil {
		return HTTPResult{
			DurationMS:     elapsedMillis(start),
			DialDurationMS: dialDuration,
			HTTPDurationMS: elapsedMillis(httpStart),
			HTTPStatus:     resp.StatusCode,
			DialMetrics:    dialMetrics,
		}, err
	}
	return HTTPResult{
		DurationMS:     elapsedMillis(start),
		DialDurationMS: dialDuration,
		HTTPDurationMS: elapsedMillis(httpStart),
		HTTPStatus:     resp.StatusCode,
		Body:           body,
		DialMetrics:    dialMetrics,
	}, nil
}

func httpResultWithElapsed(start time.Time, dialDuration int64, dialMetrics appproxy.DialMetrics) HTTPResult {
	return HTTPResult{
		DurationMS:     elapsedMillis(start),
		DialDurationMS: dialDuration,
		DialMetrics:    dialMetrics,
	}
}

func httpResultWithElapsedAndHTTP(start time.Time, dialDuration int64, dialMetrics appproxy.DialMetrics, httpStart time.Time) HTTPResult {
	result := httpResultWithElapsed(start, dialDuration, dialMetrics)
	result.HTTPDurationMS = elapsedMillis(httpStart)
	return result
}

func elapsedMillis(start time.Time) int64 {
	if start.IsZero() {
		return 0
	}
	return time.Since(start).Milliseconds()
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
