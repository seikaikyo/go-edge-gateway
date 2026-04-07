package scan

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

//go:embed all:web
var webFS embed.FS

// StaticHandler returns an http.HandlerFunc that serves the embedded React SPA.
func StaticHandler() http.HandlerFunc {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		path := chi.URLParam(r, "*")
		if path == "" || path == "/" {
			path = "index.html"
		}

		data, err := fs.ReadFile(sub, path)
		if err != nil {
			// SPA fallback: serve index.html for unknown paths
			data, err = fs.ReadFile(sub, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			path = "index.html"
		}

		ct := mime.TypeByExtension(filepath.Ext(path))
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Write(data)
	}
}
