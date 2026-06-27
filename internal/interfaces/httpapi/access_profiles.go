package httpapi

import (
	"net/http"
	"strings"

	applicationprofiles "proxygateway/internal/application/profiles"
)

const (
	validationAccessProfilePatchRequired = "策略配置不能为空"
	validationEnabledRequired            = "启用状态不能为空"
)

type AccessProfilesHandler struct {
	Auth   AuthFunc
	Create func(applicationprofiles.PatchRequest) (any, error)
	List   func(limit, offset int) (any, error)
}

type AccessProfileSubroutesHandler struct {
	Auth             AuthFunc
	Endpoint         func(host string) string
	Get              func(profileID, endpoint string) (any, error)
	Patch            func(profileID string, req applicationprofiles.PatchRequest) (any, error)
	Delete           func(profileID string) (any, error)
	ListCredentials  func(profileID, endpoint string) (any, error)
	CreateCredential func(profileID string, req ProfileCredentialCreateRequest, endpoint string) (any, error)
	PatchCredential  func(profileID, credentialID string, enabled bool) (any, error)
	DeleteCredential func(profileID, credentialID string) (any, error)
	RunAction        func(profileID, action string) (any, error)
}

type ProfileCredentialCreateRequest struct {
	Remark   string `json:"remark"`
	Password string `json:"password"`
}

type profileCredentialPatchRequest struct {
	Enabled *bool `json:"enabled"`
}

func (h AccessProfilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req applicationprofiles.PatchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		result, err := h.Create(req)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodGet:
		page, pageSize := parsePagination(r)
		result, err := h.List(pageSize, (page-1)*pageSize)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h AccessProfileSubroutesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	profileID, rest := accessProfileSubroute(r.URL.Path)
	if profileID == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	endpoint := h.endpoint(r.Host)
	if rest == "" {
		h.handleProfileRoot(w, r, profileID, endpoint)
		return
	}
	if rest == "proxy-credentials" {
		h.handleProfileCredentials(w, r, profileID, endpoint)
		return
	}
	if strings.HasPrefix(rest, "actions/") {
		action := strings.TrimPrefix(rest, "actions/")
		h.handleProfileAction(w, r, profileID, action)
		return
	}
	prefix, credentialID, ok := strings.Cut(rest, "/")
	if !ok || prefix != "proxy-credentials" || credentialID == "" || strings.Contains(credentialID, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	h.handleProfileCredential(w, r, profileID, credentialID)
}

func (h AccessProfileSubroutesHandler) handleProfileRoot(w http.ResponseWriter, r *http.Request, profileID, endpoint string) {
	switch r.Method {
	case http.MethodGet:
		result, err := h.Get(profileID, endpoint)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodPatch:
		var req applicationprofiles.PatchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.IsEmpty() {
			writeError(w, http.StatusBadRequest, validationAccessProfilePatchRequired)
			return
		}
		result, err := h.Patch(profileID, req)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodDelete:
		result, err := h.Delete(profileID)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h AccessProfileSubroutesHandler) handleProfileCredentials(w http.ResponseWriter, r *http.Request, profileID, endpoint string) {
	switch r.Method {
	case http.MethodGet:
		result, err := h.ListCredentials(profileID, endpoint)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodPost:
		var req ProfileCredentialCreateRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		result, err := h.CreateCredential(profileID, req, endpoint)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h AccessProfileSubroutesHandler) handleProfileCredential(w http.ResponseWriter, r *http.Request, profileID, credentialID string) {
	switch r.Method {
	case http.MethodPatch:
		var req profileCredentialPatchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Enabled == nil {
			writeError(w, http.StatusBadRequest, validationEnabledRequired)
			return
		}
		result, err := h.PatchCredential(profileID, credentialID, *req.Enabled)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodDelete:
		result, err := h.DeleteCredential(profileID, credentialID)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h AccessProfileSubroutesHandler) handleProfileAction(w http.ResponseWriter, r *http.Request, profileID, action string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	result, err := h.RunAction(profileID, action)
	if err != nil {
		writeStatusError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h AccessProfileSubroutesHandler) endpoint(host string) string {
	if h.Endpoint != nil {
		return h.Endpoint(host)
	}
	return host
}

func accessProfileSubroute(path string) (string, string) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/access-profiles/"), "/")
	if trimmed == "" {
		return "", ""
	}
	id, rest, ok := strings.Cut(trimmed, "/")
	if !ok {
		return id, ""
	}
	return id, strings.Trim(rest, "/")
}
