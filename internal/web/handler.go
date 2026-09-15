package web

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
	"time"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
)

//go:embed static/*
var assets embed.FS

func Handler(a *auth.Auth, version string) http.Handler {
	mux := http.NewServeMux()
	page := template.Must(template.ParseFS(assets, "static/index.html"))
	var html bytes.Buffer
	if err := page.Execute(&html, struct{ Version string }{version}); err != nil {
		panic(err)
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(html.Bytes()))
	})
	for route, file := range map[string]string{
		"GET /style.css": "static/style.css", "GET /app.js": "static/app.js",
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
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"version\":\"" + version + "\"}\n"))
	})
	a.Register(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("CDN-Cache-Control", "no-store")
		w.Header().Set("Cloudflare-CDN-Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://accounts.google.com/gsi/client; style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style; frame-src https://accounts.google.com; connect-src 'self' https://accounts.google.com; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
