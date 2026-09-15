package web

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/robbertvdzon/maceclubheemskerk/internal/auth"
	"github.com/robbertvdzon/maceclubheemskerk/internal/content"
)

// Optional isolated browser fixture, compiled only by go test. Never uses real
// credentials, production data or storage. Start explicitly with MCH_BROWSER_TEST=1.
func TestBrowserPreview(t *testing.T) {
	if os.Getenv("MCH_BROWSER_TEST") != "1" {
		t.Skip("isolated interactive preview only")
	}
	dir := t.TempDir()
	sessions, err := auth.OpenStore(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Close()
	library, err := content.Open(filepath.Join(dir, "maceclub.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	cfg := auth.Config{ClientID: "local-test-only", Origins: []string{"http://127.0.0.1:18082"}, MemberEmails: []string{"member@example.test"}}
	a := auth.New(cfg, sessions, func(context.Context, string, string, string) (auth.User, error) { return auth.User{}, nil })
	token, _, err := sessions.Create(auth.User{ID: "local-test-only", Name: "Test clublid", Email: "member@example.test"}, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(a, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", library)
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.HandleFunc("GET /fixture/login", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "mch-local-session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	t.Log("isolated preview available on port 18082")
	server := &http.Server{Addr: ":18082", Handler: mux}
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		t.Fatal(err)
	}
}
