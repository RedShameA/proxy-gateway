package app

import (
	"database/sql"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (g *Gateway) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		RequiresSetup bool      `json:"requires_setup"`
		Build         BuildInfo `json:"build"`
	}{
		RequiresSetup: !g.hasAdminCredential(),
		Build:         CurrentBuildInfo(),
	})
}

func (g *Gateway) handleAdminSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if g.hasAdminCredential() {
		writeError(w, http.StatusConflict, "admin credential already exists")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Password) < 12 {
		writeError(w, http.StatusBadRequest, validationAdminPasswordMinLength)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash password")
		return
	}
	if _, err := g.db.Exec(
		`INSERT INTO admin_credentials (id, password_hash, created_at) VALUES (1, ?, ?)`,
		string(hash),
		unixMillisNow(),
	); err != nil {
		writeError(w, http.StatusInternalServerError, "create admin credential")
		return
	}
	token, err := g.createAdminSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (g *Gateway) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	loginKey := adminLoginLimitKey(r)
	now := unixMillisNow()
	if retryAfter, ok := g.adminLogins.allow(loginKey, now); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeError(w, http.StatusTooManyRequests, "too many login attempts")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	var passwordHash string
	err := g.db.QueryRow(`SELECT password_hash FROM admin_credentials LIMIT 1`).Scan(&passwordHash)
	if errors.Is(err, sql.ErrNoRows) || bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)) != nil {
		g.adminLogins.recordFailure(loginKey, now)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load credential")
		return
	}
	token, err := g.createAdminSession()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create session")
		return
	}
	g.adminLogins.recordSuccess(loginKey)
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (g *Gateway) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !g.isAdminAuthenticated(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (g *Gateway) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !g.isAdminAuthenticated(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func (g *Gateway) hasAdminCredential() bool {
	var exists int
	_ = g.db.QueryRow(`SELECT 1 FROM admin_credentials WHERE id = 1`).Scan(&exists)
	return exists == 1
}

func (g *Gateway) createAdminSession() (string, error) {
	g.deleteExpiredAdminSessions(unixMillisNow())
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if _, err := g.db.Exec(
		`INSERT INTO admin_sessions (token_hash, created_at) VALUES (?, ?)`,
		tokenHash(token),
		unixMillisNow(),
	); err != nil {
		return "", err
	}
	return token, nil
}

func (g *Gateway) isAdminAuthenticated(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok || strings.TrimSpace(token) == "" {
		return false
	}
	hash := tokenHash(token)
	var createdAt int64
	err := g.db.QueryRow(`SELECT created_at FROM admin_sessions WHERE token_hash = ?`, hash).Scan(&createdAt)
	if err != nil {
		return false
	}
	if adminSessionExpired(createdAt, unixMillisNow()) {
		_, _ = g.db.Exec(`DELETE FROM admin_sessions WHERE token_hash = ?`, hash)
		return false
	}
	return true
}

func adminSessionExpired(createdAt, now int64) bool {
	if createdAt <= 0 {
		return true
	}
	return now-createdAt > int64(adminSessionTTL/time.Millisecond)
}

func (g *Gateway) deleteExpiredAdminSessions(now int64) {
	cutoff := now - int64(adminSessionTTL/time.Millisecond)
	_, _ = g.db.Exec(`DELETE FROM admin_sessions WHERE created_at <= ?`, cutoff)
}
