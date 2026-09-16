package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"github.com/robbertvdzon/maceclubheemskerk/internal/content"
	"html/template"
	"net/http"
	"time"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
)

//go:embed static/*
var assets embed.FS

func Handler(a *auth.Auth, version string, stores ...*content.Store) http.Handler {
	var library *content.Store
	if len(stores) > 0 {
		library = stores[0]
	}
	mux := http.NewServeMux()
	page := template.Must(template.ParseFS(assets, "static/index.html"))
	pages := []struct{ Path, Page, Title string }{{"/", "home", "Home"}, {"/fotos-en-filmpjes", "media", "Foto’s en filmpjes"}, {"/wie-zijn-wij", "about", "Wie zijn wij"}, {"/bingo", "bingo", "Lennarts bingo"}}
	for _, entry := range pages {
		var html bytes.Buffer
		if err := page.Execute(&html, struct{ Version, Page, Title string }{version, entry.Page, entry.Title}); err != nil {
			panic(err)
		}
		route := "GET " + entry.Path
		if entry.Path == "/" {
			route = "GET /{$}"
		}
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(html.Bytes()))
		})
	}
	for route, file := range map[string]string{
		"GET /style.css": "static/style.css", "GET /app.js": "static/app.js",
		"GET /bingo.js": "static/bingo.js", "GET /lennart-caricature.png": "static/lennart-caricature.png",
		"GET /video-upload.js": "static/video-upload.js",
		"GET /monitor.js":      "static/monitor.js", "GET /content.js": "static/content.js",
		"GET /favicon.svg": "static/favicon.svg", "GET /club-logo.png": "static/club-logo.png",
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
		var revision int64
		if library != nil {
			var err error
			revision, err = library.Revision(r.Context())
			if err != nil {
				http.Error(w, "temporarily unavailable", 503)
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": version, "revision": revision})
	})
	a.Register(mux)
	if library != nil {
		library.Register(mux, a)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("CDN-Cache-Control", "no-store")
		w.Header().Set("Cloudflare-CDN-Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin-allow-popups")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://accounts.google.com/gsi/client; style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style; media-src 'self' blob:; img-src 'self' blob: https://i.ytimg.com; frame-src https://accounts.google.com https://www.youtube-nocookie.com; connect-src 'self' https://accounts.google.com; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		mux.ServeHTTP(w, r)
	})
}
