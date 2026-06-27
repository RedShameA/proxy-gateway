package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const DefaultWebDistDir = "/app/web/dist"

type WebUIHandler struct {
	DistDir string
}

func (h WebUIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	distDir := h.DistDir
	if distDir == "" {
		distDir = DefaultWebDistDir
	}
	indexPath := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>Proxy Gateway</title><div id=\"root\">Proxy Gateway</div>"))
		return
	}
	cleanPath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if cleanPath == "." {
		http.ServeFile(w, r, indexPath)
		return
	}
	filePath := filepath.Join(distDir, cleanPath)
	rel, err := filepath.Rel(distDir, filePath)
	if err != nil || strings.HasPrefix(rel, "..") {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	if stat, err := os.Stat(filePath); err == nil && !stat.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}
	http.ServeFile(w, r, indexPath)
}
