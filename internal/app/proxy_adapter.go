package app

import (
	"context"
	"net"
	"net/http"
	"strings"

	appproxy "proxygateway/internal/application/proxy"
	proxyiface "proxygateway/internal/interfaces/proxy"
)

func (g *Gateway) handleHTTPProxy(w http.ResponseWriter, r *http.Request) {
	g.httpProxyAdapter().ServeHTTP(w, r)
}

func (g *Gateway) handleSOCKS5Conn(conn net.Conn) {
	g.socks5ProxyAdapter().ServeConn(conn)
}

func (g *Gateway) httpProxyAdapter() proxyiface.HTTPAdapter[proxyCredentialRecord, selectedProxyPath] {
	return proxyiface.HTTPAdapter[proxyCredentialRecord, selectedProxyPath]{
		Authenticate:                 g.authenticateProxyCredential,
		SelectPath:                   g.proxyPathForCredential,
		PathProfileIdentifier:        func(path selectedProxyPath) string { return path.ProfileIdentifier },
		PathFailureProfileIdentifier: g.pathFailureProfileIdentifier,
		Dial:                         g.dialProxyPath,
		RecordFailure:                g.requestLogService.RecordFailure,
		StartRequest:                 g.requestLogService.Start,
		FinishRequest:                g.requestLogService.Finish,
	}
}

func (g *Gateway) socks5ProxyAdapter() proxyiface.SOCKS5Adapter[proxyCredentialRecord, selectedProxyPath] {
	return proxyiface.SOCKS5Adapter[proxyCredentialRecord, selectedProxyPath]{
		Authenticate:        g.authenticateProxyCredential,
		SelectPath:          g.proxyPathForCredential,
		CredentialProfileID: func(credential proxyCredentialRecord) string { return credential.ProfileID },
		Dial:                g.dialProxyPath,
		StartRequest:        g.requestLogService.Start,
		FinishRequest:       g.requestLogService.Finish,
		Logger:              g.log(),
	}
}

func (g *Gateway) authenticateProxyCredential(username, password string) proxyiface.AuthResult[proxyCredentialRecord] {
	rec, failureStage, errorText, ok := g.loadProxyCredentialForProxy(username, password)
	if !ok {
		return proxyiface.AuthResult[proxyCredentialRecord]{
			Failure: appproxy.Failure{Stage: failureStage, Error: errorText},
		}
	}
	return proxyiface.AuthResult[proxyCredentialRecord]{Credential: rec, OK: true}
}

func (g *Gateway) pathFailureProfileIdentifier(credential proxyCredentialRecord, fallback string) string {
	identifier, _, _ := g.profileCredentialRepo.LoadProfileIdentifier(context.Background(), credential.ProfileID)
	if strings.TrimSpace(identifier) != "" {
		return identifier
	}
	if strings.TrimSpace(credential.ProfileID) != "" {
		return credential.ProfileID
	}
	return fallback
}
