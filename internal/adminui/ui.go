// SPDX-License-Identifier: AGPL-3.0-only
package adminui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// A source-controlled preparation page keeps Go-only tests/builds valid before
// npm ci && npm run build. Release builds always replace it with the dashboard.
//
//go:embed dist prepare.html
var files embed.FS

func Handler() http.Handler {
	root, _ := fs.Sub(files, "dist")
	handler := http.StripPrefix("/admin/", http.FileServer(http.FS(root)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(root, "index.html"); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			raw, _ := files.ReadFile("prepare.html")
			w.Write(raw)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/admin/")
		area := strings.SplitN(path, "/", 2)[0]
		if area == "tenants" || area == "users" || area == "status" {
			// Browser routes reload through the SPA; assets/source retain normal
			// file serving and missing assets must still return 404.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			raw, _ := fs.ReadFile(root, "index.html")
			if r.Method != http.MethodHead {
				_, _ = w.Write(raw)
			}
			return
		}
		handler.ServeHTTP(w, r)
	})
}
func SourceHandler() http.Handler {
	root, _ := fs.Sub(files, "dist")
	return http.FileServer(http.FS(root))
}
