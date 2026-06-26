package app

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type outboundGETRequest struct {
	TargetHost string
	Request    *http.Request
	HTTPS      bool
	ServerName string
}

func (g *Gateway) nodeEngine() nodeProtocolEngine {
	return g.protocolEngine
}

func (g *Gateway) dialProxyPath(path selectedProxyPath, target string) (net.Conn, error) {
	timeouts := g.proxyDialTimeouts()
	if path.FrontNode.ID != "" && path.ExitNode.ID != "" {
		return g.nodeEngine().DialChain(path.FrontNode, path.ExitNode, target, timeouts)
	}
	return g.nodeEngine().DialNode(path.Node, target, timeouts)
}

func (g *Gateway) proxyDialTimeouts() dialTimeouts {
	settings, err := g.loadEvaluationSettings()
	if err != nil {
		settings = normalizeEvaluationSettings(evaluationSettings{})
	}
	settings = normalizeEvaluationSettings(settings)
	return dialTimeouts{
		ConnectTimeout: time.Duration(settings.ConnectTimeoutSeconds) * time.Second,
	}
}

func (g *Gateway) dialViaChain(frontNode, exitNode nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	return g.nodeEngine().DialChain(frontNode, exitNode, target, timeouts)
}

func (g *Gateway) dialViaNode(node nodeRecord, target string, timeouts dialTimeouts) (net.Conn, error) {
	return g.nodeEngine().DialNode(node, target, timeouts)
}

func buildOutboundGET(rawURL string) (string, *http.Request, error) {
	outbound, err := buildOutboundGETRequest(rawURL)
	if err != nil {
		return "", nil, err
	}
	return outbound.TargetHost, outbound.Request, nil
}

func buildOutboundGETRequest(rawURL string) (outboundGETRequest, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return outboundGETRequest{}, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return outboundGETRequest{}, errors.New(validationTestURLScheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return outboundGETRequest{}, errors.New(validationTestURLHostRequired)
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
		return outboundGETRequest{}, err
	}
	req.Host = u.Host
	serverName := u.Hostname()
	return outboundGETRequest{
		TargetHost: targetHost,
		Request:    req,
		HTTPS:      u.Scheme == "https",
		ServerName: serverName,
	}, nil
}

func wrapOutboundGETConn(conn net.Conn, outbound outboundGETRequest) (net.Conn, error) {
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
