package httpapi

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	appadmin "proxygateway/internal/application/admin"
)

const (
	adminPasswordMinLengthMessage    = "管理员密码至少 12 个字符"
	newAdminPasswordMinLengthMessage = "新密码至少 12 个字符"
)

type SetupStatusHandler struct {
	Admin appadmin.Service
	Build any
}

func AdminAuth(admin appadmin.Service) AuthFunc {
	return func(w http.ResponseWriter, r *http.Request) bool {
		auth := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(auth, "Bearer ")
		if !ok || strings.TrimSpace(token) == "" || !admin.Authenticated(r.Context(), token) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return false
		}
		return true
	}
}

func (h SetupStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	requiresSetup, err := h.Admin.RequiresSetup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load credential")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		RequiresSetup bool `json:"requires_setup"`
		Build         any  `json:"build"`
	}{
		RequiresSetup: requiresSetup,
		Build:         h.Build,
	})
}

type AdminSetupHandler struct {
	Admin appadmin.Service
}

func (h AdminSetupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	token, err := h.Admin.Setup(r.Context(), req.Password)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"token": token})
	case errors.Is(err, appadmin.ErrCredentialExists):
		writeError(w, http.StatusConflict, "admin credential already exists")
	case errors.Is(err, appadmin.ErrPasswordTooShort):
		writeError(w, http.StatusBadRequest, adminPasswordMinLengthMessage)
	default:
		writeError(w, http.StatusInternalServerError, setupErrorMessage(err))
	}
}

type AdminLoginHandler struct {
	Admin   appadmin.Service
	Limiter *appadmin.LoginLimiter
}

func (h AdminLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	loginKey := adminLoginLimitKey(r)
	nowMillis := time.Now().UnixMilli()
	if h.Admin.Now != nil {
		nowMillis = h.Admin.Now()
	}
	if retryAfter, ok := h.Limiter.Allow(loginKey, nowMillis); !ok {
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
	token, err := h.Admin.Login(r.Context(), req.Password)
	if err != nil {
		if errors.Is(err, appadmin.ErrInvalidCredentials) {
			h.Limiter.RecordFailure(loginKey, nowMillis)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "load credential")
		return
	}
	h.Limiter.RecordSuccess(loginKey)
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

type AdminMeHandler struct {
	Auth AuthFunc
}

func (h AdminMeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

type AdminPasswordHandler struct {
	Auth  AuthFunc
	Admin appadmin.Service
}

func (h AdminPasswordHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	err := h.Admin.ChangePassword(r.Context(), req.CurrentPassword, req.NewPassword)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
	case errors.Is(err, appadmin.ErrPasswordTooShort):
		writeError(w, http.StatusBadRequest, newAdminPasswordMinLengthMessage)
	case errors.Is(err, appadmin.ErrCurrentPasswordIncorrect):
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
	default:
		writeError(w, http.StatusInternalServerError, "update password")
	}
}

func setupErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return "create admin credential"
}

func adminLoginLimitKey(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(token)
}
