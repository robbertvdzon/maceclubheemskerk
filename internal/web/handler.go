package web

import (
	"bytes"
	"embed"
	"net/http"
	"time"
)

//go:embed static/index.html static/style.css
var assets embed.FS

// Handler serves the site without a database or runtime filesystem dependencies.
func Handler() http.Handler {
	mux := http.NewServeMux()
	for route, file := range map[string]string{
		"GET /{$}":       "static/index.html",
		"GET /style.css": "static/style.css",
	} {
		body, err := assets.ReadFile(file)
		if err != nil {
			panic(err)
		}
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, file, time.Time{}, bytes.NewReader(body))
		})
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
