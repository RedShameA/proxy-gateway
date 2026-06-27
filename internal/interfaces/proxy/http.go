package proxy

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"time"

	appproxy "proxygateway/internal/application/proxy"
)

type AuthResult[C any] struct {
	Credential C
	Failure    appproxy.Failure
	OK         bool
}

type HTTPAdapter[C any, P any] struct {
	Authenticate                 func(username, password string) AuthResult[C]
	SelectPath                   func(C) (P, error)
	CredentialProfileID          func(C) string
	PathProfileIdentifier        func(P) string
	PathFailureProfileIdentifier func(C, string) string
	Dial                         func(P, string) (net.Conn, error)
	RecordFailure                func(targetHost, profileIdentifier, stage, errorText string, statusCode int, startedAt time.Time)
	StartRequest                 func(P, string, time.Time) string
	FinishRequest                func(logID string, success bool, failureStage, errorText string, httpStatus int, durationMS, ingressBytes, egressBytes int64)
}

func (h HTTPAdapter[C, P]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	targetHost := RequestTargetHost(r)
	profileIdentifier := RequestProfileIdentifier(r)
	credential, failure, ok := h.authenticateRequest(w, r)
	if !ok {
		h.recordFailure(targetHost, profileIdentifier, failure.Stage, failure.Error, http.StatusProxyAuthRequired, start)
		return
	}
	path, err := h.SelectPath(credential)
	if err != nil {
		failure := appproxy.ClassifyProxyPathFailure(err.Error())
		h.recordFailure(targetHost, h.pathFailureProfileIdentifier(credential, profileIdentifier), failure.Stage, failure.Error, http.StatusBadGateway, start)
		WriteJSONError(w, http.StatusBadGateway, failure.Error)
		return
	}
	if r.Method == http.MethodConnect {
		h.handleConnect(w, r, path, start)
		return
	}
	if !r.URL.IsAbs() {
		h.recordFailure(targetHost, h.pathProfileIdentifier(path), appproxy.FailureStageUpstream, "absolute proxy URL required", http.StatusBadRequest, start)
		WriteJSONError(w, http.StatusBadRequest, "absolute proxy URL required")
		return
	}
	h.handleHTTPRequest(w, r, path, start)
}

func (h HTTPAdapter[C, P]) authenticateRequest(w http.ResponseWriter, r *http.Request) (C, appproxy.Failure, bool) {
	auth := r.Header.Get("Proxy-Authorization")
	username, password, ok := ParseBasicProxyAuthorization(auth)
	if !ok {
		w.Header().Set("Proxy-Authenticate", "Basic realm=\"Proxy Gateway\"")
		failure := h.authenticationFailure(r)
		WriteJSONError(w, http.StatusProxyAuthRequired, failure.Error)
		var zero C
		return zero, failure, false
	}
	result := h.Authenticate(username, password)
	if !result.OK {
		WriteJSONError(w, http.StatusProxyAuthRequired, result.Failure.Error)
		var zero C
		return zero, result.Failure, false
	}
	return result.Credential, appproxy.Failure{}, true
}

func (h HTTPAdapter[C, P]) authenticationFailure(r *http.Request) appproxy.Failure {
	if strings.TrimSpace(r.Header.Get("Proxy-Authorization")) == "" {
		return appproxy.MissingProxyAuthenticationFailure()
	}
	return appproxy.InvalidProxyAuthenticationFailure()
}

func (h HTTPAdapter[C, P]) handleConnect(w http.ResponseWriter, r *http.Request, path P, start time.Time) {
	logID := h.startRequest(path, r.Host, start)
	targetConn, err := h.Dial(path, r.Host)
	if err != nil {
		h.finishRequest(logID, false, appproxy.FailureStageDial, err.Error(), 0, elapsedMilliseconds(start), 0, 0)
		WriteJSONError(w, http.StatusBadGateway, "connect target")
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		_ = targetConn.Close()
		h.finishRequest(logID, false, appproxy.FailureStageProxyHandshake, "hijack unsupported", 0, elapsedMilliseconds(start), 0, 0)
		WriteJSONError(w, http.StatusInternalServerError, "hijack unsupported")
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		_ = targetConn.Close()
		h.finishRequest(logID, false, appproxy.FailureStageProxyHandshake, "hijack failed", 0, elapsedMilliseconds(start), 0, 0)
		return
	}
	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	ingressBytes, egressBytes := PipeConns(clientConn, targetConn)
	h.finishRequest(logID, true, "", "", 0, elapsedMilliseconds(start), ingressBytes, egressBytes)
}

func (h HTTPAdapter[C, P]) handleHTTPRequest(w http.ResponseWriter, r *http.Request, path P, start time.Time) {
	targetHost := AbsoluteProxyURLTargetHost(r)
	logID := h.startRequest(path, targetHost, start)
	targetConn, err := h.Dial(path, targetHost)
	if err != nil {
		h.finishRequest(logID, false, appproxy.FailureStageDial, err.Error(), 0, elapsedMilliseconds(start), 0, 0)
		WriteJSONError(w, http.StatusBadGateway, "connect target")
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
		h.finishRequest(logID, false, appproxy.FailureStageUpstream, "write upstream request", 0, elapsedMilliseconds(start), 0, 0)
		WriteJSONError(w, http.StatusBadGateway, "write upstream request")
		return
	}
	resp, err := http.ReadResponse(bufio.NewReader(targetConn), outReq)
	if err != nil {
		h.finishRequest(logID, false, appproxy.FailureStageUpstream, "read upstream response", 0, elapsedMilliseconds(start), 0, 0)
		WriteJSONError(w, http.StatusBadGateway, "read upstream response")
		return
	}
	defer resp.Body.Close()
	egressBytes := CopyResponse(w, resp)
	h.finishRequest(logID, true, "", "", resp.StatusCode, elapsedMilliseconds(start), 0, egressBytes)
}

func RequestTargetHost(r *http.Request) string {
	if r.Method == http.MethodConnect {
		return r.Host
	}
	if r.URL.IsAbs() {
		return AbsoluteProxyURLTargetHost(r)
	}
	return r.Host
}

func AbsoluteProxyURLTargetHost(r *http.Request) string {
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

func RequestProfileIdentifier(r *http.Request) string {
	username, _, ok := ParseBasicProxyAuthorization(r.Header.Get("Proxy-Authorization"))
	if !ok {
		return ""
	}
	return strings.TrimSpace(username)
}

func (h HTTPAdapter[C, P]) pathProfileIdentifier(path P) string {
	if h.PathProfileIdentifier == nil {
		return ""
	}
	return h.PathProfileIdentifier(path)
}

func (h HTTPAdapter[C, P]) pathFailureProfileIdentifier(credential C, fallback string) string {
	if h.PathFailureProfileIdentifier == nil {
		return fallback
	}
	return h.PathFailureProfileIdentifier(credential, fallback)
}

func (h HTTPAdapter[C, P]) recordFailure(targetHost, profileIdentifier, stage, errorText string, statusCode int, startedAt time.Time) {
	if h.RecordFailure != nil {
		h.RecordFailure(targetHost, profileIdentifier, stage, errorText, statusCode, startedAt)
	}
}

func (h HTTPAdapter[C, P]) startRequest(path P, targetHost string, startedAt time.Time) string {
	if h.StartRequest == nil {
		return ""
	}
	return h.StartRequest(path, targetHost, startedAt)
}

func (h HTTPAdapter[C, P]) finishRequest(logID string, success bool, failureStage, errorText string, httpStatus int, durationMS, ingressBytes, egressBytes int64) {
	if h.FinishRequest != nil {
		h.FinishRequest(logID, success, failureStage, errorText, httpStatus, durationMS, ingressBytes, egressBytes)
	}
}

func elapsedMilliseconds(start time.Time) int64 {
	elapsed := time.Since(start).Milliseconds()
	if elapsed <= 0 {
		return 1
	}
	return elapsed
}
