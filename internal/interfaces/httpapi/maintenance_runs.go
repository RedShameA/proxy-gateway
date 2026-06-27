package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	appmaintenance "proxygateway/internal/application/maintenance"
)

type MaintenanceRunService interface {
	List(ctx context.Context, filter appmaintenance.ListFilter) (appmaintenance.ListResult, error)
	Load(ctx context.Context, id string) (appmaintenance.Run, error)
}

type MaintenanceRunsHandler struct {
	Auth    AuthFunc
	Service MaintenanceRunService
}

func (h MaintenanceRunsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	if r.URL.Path != "/api/maintenance/runs" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	page, pageSize := parsePagination(r)
	filter := maintenanceRunFilters(r)
	filter.Page = page
	filter.PageSize = pageSize
	list, err := h.Service.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list maintenance runs")
		return
	}
	items := make([]map[string]any, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, appmaintenance.RunToMap(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"total":     list.Total,
		"page":      page,
		"page_size": pageSize,
	})
}

type MaintenanceRunDetailHandler struct {
	Auth    AuthFunc
	Service MaintenanceRunService
}

func (h MaintenanceRunDetailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Auth != nil && !h.Auth(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/maintenance/runs/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	run, err := h.Service.Load(r.Context(), id)
	if err != nil {
		if errors.Is(err, appmaintenance.ErrRunNotFound) {
			writeError(w, http.StatusNotFound, "maintenance run not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "load maintenance run")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": appmaintenance.RunToMap(run)})
}

func maintenanceRunFilters(r *http.Request) appmaintenance.ListFilter {
	query := r.URL.Query()
	return appmaintenance.ListFilter{
		RunType:  strings.TrimSpace(query.Get("run_type")),
		TargetID: strings.TrimSpace(query.Get("target_id")),
		State:    strings.TrimSpace(query.Get("state")),
		Result:   strings.TrimSpace(query.Get("result")),
	}
}
