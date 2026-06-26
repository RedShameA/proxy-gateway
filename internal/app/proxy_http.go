package app

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	appproxy "proxygateway/internal/application/proxy"
)

func (g *Gateway) handleHTTPProxy(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	targetHost := proxyRequestTargetHost(r)
	profileIdentifier := proxyRequestProfileIdentifier(r)
	credential, failureStage, errorText, ok := g.proxyCredentialForRequest(w, r)
	if !ok {
		g.recordProxyFailure(targetHost, profileIdentifier, failureStage, errorText, http.StatusProxyAuthRequired, start)
		return
	}
	path, err := g.proxyPathForCredential(credential)
	if err != nil {
		failure := appproxy.ClassifyProxyPathFailure(err.Error())
		g.recordProxyFailure(targetHost, g.pathFailureProfileIdentifier(credential.ProfileID, profileIdentifier), failure.Stage, failure.Error, http.StatusBadGateway, start)
		writeError(w, http.StatusBadGateway, failure.Error)
		return
	}
	if r.Method == http.MethodConnect {
		g.handleProxyCONNECT(w, r, path, start)
		return
	}
	if !r.URL.IsAbs() {
		g.recordProxyFailure(targetHost, path.ProfileIdentifier, requestFailureStageUpstream, "absolute proxy URL required", http.StatusBadRequest, start)
		writeError(w, http.StatusBadRequest, "absolute proxy URL required")
		return
	}
	g.handleProxyHTTPRequest(w, r, path, start)
}

func (g *Gateway) handleProxyCONNECT(w http.ResponseWriter, r *http.Request, path selectedProxyPath, start time.Time) {
	logID := g.startProxyRequest(path, r.Host, start)
	targetConn, err := g.dialProxyPath(path, r.Host)
	if err != nil {
		g.finishProxyRequest(logID, false, requestFailureStageDial, err.Error(), 0, elapsedMilliseconds(start), 0, 0)
		writeError(w, http.StatusBadGateway, "connect target")
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = targetConn.Close()
		g.finishProxyRequest(logID, false, requestFailureStageProxyHandshake, "hijack unsupported", 0, elapsedMilliseconds(start), 0, 0)
		writeError(w, http.StatusInternalServerError, "hijack unsupported")
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		_ = targetConn.Close()
		g.finishProxyRequest(logID, false, requestFailureStageProxyHandshake, "hijack failed", 0, elapsedMilliseconds(start), 0, 0)
		return
	}
	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	ingressBytes, egressBytes := pipeConns(clientConn, targetConn)
	g.finishProxyRequest(logID, true, "", "", 0, elapsedMilliseconds(start), ingressBytes, egressBytes)
}

func (g *Gateway) handleProxyHTTPRequest(w http.ResponseWriter, r *http.Request, path selectedProxyPath, start time.Time) {
	targetHost := absoluteProxyURLTargetHost(r)
	logID := g.startProxyRequest(path, targetHost, start)
	targetConn, err := g.dialProxyPath(path, targetHost)
	if err != nil {
		g.finishProxyRequest(logID, false, requestFailureStageDial, err.Error(), 0, elapsedMilliseconds(start), 0, 0)
		writeError(w, http.StatusBadGateway, "connect target")
		return
	}
	defer targetConn.Close()

	outReq := r.Clone(r.Context())
	outReq.RequestURI = ""
	outReq.URL.Scheme = ""
	outReq.URL.Host = ""
	outReq.Host = r.URL.Host
	outReq.Header.Del("Proxy-Authorization")
	if err := outReq.Write(targetConn); err != nil {
		g.finishProxyRequest(logID, false, requestFailureStageUpstream, "write upstream request", 0, elapsedMilliseconds(start), 0, 0)
		writeError(w, http.StatusBadGateway, "write upstream request")
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(targetConn), outReq)
	if err != nil {
		g.finishProxyRequest(logID, false, requestFailureStageUpstream, "read upstream response", 0, elapsedMilliseconds(start), 0, 0)
		writeError(w, http.StatusBadGateway, "read upstream response")
		return
	}
	defer resp.Body.Close()
	egressBytes := copyResponse(w, resp)
	g.finishProxyRequest(logID, true, "", "", resp.StatusCode, elapsedMilliseconds(start), 0, egressBytes)
}

func proxyRequestTargetHost(r *http.Request) string {
	if r.Method == http.MethodConnect {
		return r.Host
	}
	if r.URL.IsAbs() {
		return absoluteProxyURLTargetHost(r)
	}
	return r.Host
}

func absoluteProxyURLTargetHost(r *http.Request) string {
	host := r.URL.Hostname()
	port := r.URL.Port()
	if port == "" {
		port = "80"
		if r.URL.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(host, port)
}

func proxyRequestProfileIdentifier(r *http.Request) string {
	username, _, ok := parseBasicProxyAuthorization(r.Header.Get("Proxy-Authorization"))
	if !ok {
		return ""
	}
	return strings.TrimSpace(username)
}

func (g *Gateway) pathFailureProfileIdentifier(profileID, fallback string) string {
	var identifier string
	_ = g.db.QueryRow(`SELECT profile_identifier FROM access_profiles WHERE id = ?`, profileID).Scan(&identifier)
	if strings.TrimSpace(identifier) != "" {
		return identifier
	}
	if strings.TrimSpace(profileID) != "" {
		return profileID
	}
	return fallback
}

func pipeConns(a, b net.Conn) (int64, int64) {
	type pipeResult struct {
		name  string
		bytes int64
	}
	done := make(chan pipeResult, 2)
	go func() {
		n, _ := io.Copy(a, b)
		_ = a.Close()
		_ = b.Close()
		done <- pipeResult{name: "egress", bytes: n}
	}()
	go func() {
		n, _ := io.Copy(b, a)
		_ = b.Close()
		_ = a.Close()
		done <- pipeResult{name: "ingress", bytes: n}
	}()
	var ingressBytes, egressBytes int64
	for i := 0; i < 2; i++ {
		result := <-done
		if result.name == "ingress" {
			ingressBytes = result.bytes
		} else {
			egressBytes = result.bytes
		}
	}
	return ingressBytes, egressBytes
}

func copyResponse(w http.ResponseWriter, resp *http.Response) int64 {
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	n, _ := io.Copy(w, resp.Body)
	return n
}
