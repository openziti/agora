package ui

import (
	"bytes"
	"io/fs"
	"net/http"
	pathpkg "path"
	"strings"
	"time"
)

// Middleware routes Agora API requests to apiHandler and serves the
// embedded React SPA for every non-API path.
func Middleware(apiHandler http.Handler) http.Handler {
	if apiHandler == nil {
		apiHandler = http.NotFoundHandler()
	}
	dist := distFS()
	files := http.FileServer(http.FS(dist))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			apiHandler.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if hasStaticFile(dist, r.URL.Path) {
			files.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, dist)
	})
}

func isAPIPath(p string) bool {
	return p == "/v1" || strings.HasPrefix(p, "/v1/")
}

func hasStaticFile(dist fs.FS, requestPath string) bool {
	name := strings.TrimPrefix(pathpkg.Clean(requestPath), "/")
	if name == "." || name == "" {
		return false
	}
	info, err := fs.Stat(dist, name)
	return err == nil && !info.IsDir()
}

func serveIndex(w http.ResponseWriter, r *http.Request, dist fs.FS) {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(index))
}
