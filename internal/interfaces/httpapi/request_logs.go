package httpapi

import (
	"net/http"
	"strings"
	"time"

	appproxy "proxygateway/internal/application/proxy"
)

type RequestLogsHandler struct {
	Auth AuthFunc
	Repo appproxy.RequestLogRepository
	Now  func() int64
}

func (h RequestLogsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	page, pageSize := parsePagination(r)
	list, err := h.Repo.List(r.Context(), requestLogFilters(r, page, pageSize))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list request logs")
		return
	}
	nowMillis := h.nowMillis()
	logs := make([]map[string]any, 0, len(list.Items))
	for _, item := range list.Items {
		logs = append(logs, appproxy.RequestLogEntryToMap(item, nowMillis))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": logs, "request_logs": logs, "total": list.Total, "page": page, "page_size": pageSize})
}

func (h RequestLogsHandler) nowMillis() int64 {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now().UnixMilli()
}

func requestLogFilters(r *http.Request, page, pageSize int) appproxy.RequestLogListFilter {
	query := r.URL.Query()
	result := strings.ToLower(strings.TrimSpace(query.Get("success")))
	if result == "" {
		result = strings.ToLower(strings.TrimSpace(query.Get("result")))
	}
	return appproxy.RequestLogListFilter{
		AccessProfile: strings.TrimSpace(firstNonEmpty(query.Get("access_profile_id"), query.Get("access_profile"))),
		Credential:    strings.TrimSpace(firstNonEmpty(query.Get("credential_id"), query.Get("proxy_credential_id"), query.Get("proxy_credential"))),
		NodeID:        strings.TrimSpace(query.Get("node_id")),
		Target:        strings.TrimSpace(query.Get("target")),
		State:         strings.ToLower(strings.TrimSpace(query.Get("state"))),
		Result:        result,
		Page:          page,
		PageSize:      pageSize,
	}
}

func parsePagination(r *http.Request) (page, pageSize int) {
	page = 1
	pageSize = 10
	if v := r.URL.Query().Get("page"); v != "" {
		if n := parseInt(v); n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n := parseInt(v); n > 0 && n <= 100 {
			pageSize = n
		}
	}
	return page, pageSize
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
