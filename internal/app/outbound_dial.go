package app

import (
	"net"
	"time"
)

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
