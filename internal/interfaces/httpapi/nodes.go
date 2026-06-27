package httpapi

import (
	"net/http"
	"strings"

	applicationnodes "proxygateway/internal/application/nodes"
)

const (
	validationNodeNameTypeRequired = "节点名称和协议不能为空"
	validationNodeEndpointRequired = "节点服务器不能为空，端口需为 1-65535"
)

type NodesHandler struct {
	Auth   AuthFunc
	Create func(NodeCreateRequest) (any, error)
	List   func(applicationnodes.ListFilter) (any, error)
}

type NodeSubroutesHandler struct {
	Auth   AuthFunc
	Get    func(nodeID string) (any, error)
	Patch  func(nodeID string, req NodePatchRequest) (any, error)
	Delete func(nodeID string) error
}

type NodeCreateRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	RawJSON    string `json:"raw_json"`
	ImportText string `json:"import_text"`
}

type NodePatchRequest struct {
	Enabled    *bool   `json:"enabled"`
	Name       *string `json:"name"`
	Type       *string `json:"type"`
	Server     *string `json:"server"`
	ServerPort *int    `json:"server_port"`
	Username   *string `json:"username"`
	Password   *string `json:"password"`
	ImportText *string `json:"import_text"`
}

func (h NodesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req NodeCreateRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(req.ImportText) == "" {
			if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Type) == "" {
				writeError(w, http.StatusBadRequest, validationNodeNameTypeRequired)
				return
			}
			if req.Type != "direct" && (strings.TrimSpace(req.Server) == "" || req.ServerPort <= 0 || req.ServerPort > 65535) {
				writeError(w, http.StatusBadRequest, validationNodeEndpointRequired)
				return
			}
		}
		result, err := h.Create(req)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodGet:
		page, pageSize := parsePagination(r)
		filter := nodeListFilter(r, pageSize, (page-1)*pageSize)
		result, err := h.List(filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list nodes")
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h NodeSubroutesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	nodeID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	if nodeID == "" || strings.Contains(nodeID, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := h.Get(nodeID)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodPatch:
		var req NodePatchRequest
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Enabled == nil && !req.hasManualNodeFields() {
			writeError(w, http.StatusBadRequest, "enabled or manual node fields are required")
			return
		}
		result, err := h.Patch(nodeID, req)
		if err != nil {
			writeStatusError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodDelete:
		if err := h.Delete(nodeID); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (req NodePatchRequest) hasManualNodeFields() bool {
	return req.ImportText != nil || req.Name != nil || req.Type != nil || req.Server != nil || req.ServerPort != nil || req.Username != nil || req.Password != nil
}

func nodeListFilter(r *http.Request, limit, offset int) applicationnodes.ListFilter {
	query := r.URL.Query()
	countryFilter := normalizeEgressCountryValue(query.Get("egress_country"))
	if countryFilter == "" {
		countryFilter = normalizeEgressCountryValue(query.Get("country"))
	}
	var usable *bool
	usableFilter := strings.ToLower(strings.TrimSpace(query.Get("usable")))
	switch usableFilter {
	case "true", "1":
		value := true
		usable = &value
	case "false", "0":
		value := false
		usable = &value
	}
	return applicationnodes.ListFilter{
		Name:          query.Get("name"),
		EgressCountry: countryFilter,
		Protocol:      query.Get("protocol"),
		SourceID:      query.Get("source_id"),
		SourceType:    query.Get("source_type"),
		State:         query.Get("state"),
		Usable:        usable,
		Limit:         limit,
		Offset:        offset,
	}
}

func normalizeEgressCountryValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "__unknown__") {
		return "__unknown__"
	}
	return strings.ToUpper(value)
}
