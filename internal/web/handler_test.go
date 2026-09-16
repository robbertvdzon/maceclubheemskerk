package web

import (
	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicPageCacheHeadersVersionAndProtectedAccount(t *testing.T) {
	version := strings.Repeat("a", 64)
	h := Handler(auth.New(auth.Config{}, nil, nil), version)
	for _, path := range []string{"/", "/style.css", "/app.js", "/api/version", "/api/auth/me", "/healthz"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || w.Header().Get("Cloudflare-CDN-Cache-Control") != "no-store" {
			t.Fatal("cacheable response", path)
		}
		if path == "/" && (!strings.Contains(w.Body.String(), version) || !strings.Contains(w.Body.String(), "Inloggen")) {
			t.Fatal("missing version/login")
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/account", nil))
	if w.Code != 401 {
		t.Fatal("anonymous protected data")
	}
}

func TestSeparatePublicPages(t *testing.T) {
	h := Handler(auth.New(auth.Config{}, nil, nil), strings.Repeat("b", 64))
	markers := map[string]string{"/": "home-links", "/fotos-en-filmpjes": `id="media-grid"`, "/wie-zijn-wij": `class="members"`, "/bingo": `id="bingo-grid"`}
	for path, marker := range markers {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, marker) {
			t.Fatal("missing page", path, marker)
		}
		for other, m := range markers {
			if other != path && strings.Contains(body, m) {
				t.Fatal("pages combined", path, other)
			}
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("cached page", path)
		}
	}
}
