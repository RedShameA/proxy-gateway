package app

import (
	"encoding/base64"
	"net"
	"net/http"
	"strings"

	appproxy "proxygateway/internal/application/proxy"
)

func (g *Gateway) proxyCredentialForRequest(w http.ResponseWriter, r *http.Request) (proxyCredentialRecord, string, string, bool) {
	auth := r.Header.Get("Proxy-Authorization")
	username, password, ok := parseBasicProxyAuthorization(auth)
	if !ok {
		w.Header().Set("Proxy-Authenticate", "Basic realm=\"Proxy Gateway\"")
		if strings.TrimSpace(auth) == "" {
			failure := appproxy.MissingProxyAuthenticationFailure()
			writeError(w, http.StatusProxyAuthRequired, failure.Error)
			return proxyCredentialRecord{}, failure.Stage, failure.Error, false
		}
		failure := appproxy.InvalidProxyAuthenticationFailure()
		writeError(w, http.StatusProxyAuthRequired, failure.Error)
		return proxyCredentialRecord{}, failure.Stage, failure.Error, false
	}
	rec, failureStage, errorText, ok := g.loadProxyCredentialForProxy(username, password)
	if !ok {
		writeError(w, http.StatusProxyAuthRequired, "invalid proxy credentials")
		return proxyCredentialRecord{}, failureStage, errorText, false
	}
	return rec, "", "", true
}

func parseBasicProxyAuthorization(auth string) (string, string, bool) {
	fields := strings.Fields(auth)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Basic") {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil {
		return "", "", false
	}
	username, password, ok := strings.Cut(string(raw), ":")
	if !ok {
		return "", "", false
	}
	return username, password, true
}

func (g *Gateway) proxyCredentialForUsernamePassword(conn net.Conn, username, password string) (proxyCredentialRecord, bool) {
	rec, _, _, ok := g.loadProxyCredentialForProxy(username, password)
	if !ok {
		_, _ = conn.Write([]byte{0x01, 0x01})
		return proxyCredentialRecord{}, false
	}
	_, _ = conn.Write([]byte{0x01, 0x00})
	return rec, true
}

func (g *Gateway) loadProxyCredential(username, password string) (proxyCredentialRecord, bool) {
	rec, _, _, ok := g.loadProxyCredentialForProxy(username, password)
	return rec, ok
}

func (g *Gateway) loadProxyCredentialForProxy(username, password string) (proxyCredentialRecord, string, string, bool) {
	username = strings.TrimSpace(username)
	var profileID string
	err := g.db.QueryRow(
		`SELECT id FROM access_profiles WHERE profile_identifier = ?`,
		username,
	).Scan(&profileID)
	if err != nil {
		failure := appproxy.AccessProfileNotFoundFailure()
		return proxyCredentialRecord{}, failure.Stage, failure.Error, false
	}

	var rec proxyCredentialRecord
	err = g.db.QueryRow(
		`SELECT c.id, c.remark, c.profile_id
		   FROM proxy_credentials c
		  WHERE c.profile_id = ?
		    AND c.password = ?
		    AND c.enabled = 1`,
		profileID,
		password,
	).Scan(&rec.ID, &rec.Remark, &rec.ProfileID)
	if err != nil {
		failure := appproxy.InvalidProxyCredentialsFailure()
		return proxyCredentialRecord{}, failure.Stage, failure.Error, false
	}
	now := unixMillisNow()
	_, _ = g.db.Exec(`UPDATE proxy_credentials SET last_used_at = ? WHERE id = ? AND last_used_at < ?`, now, rec.ID, now-60_000)
	return rec, "", "", true
}
