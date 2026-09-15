package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionSurvivesRestartRenewsAndRevokes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.now = func() time.Time { return now }
	token, first, err := s.Create(User{ID: "123", Name: "Test", Email: "test@example.com"}, "")
	if err != nil {
		t.Fatal(err)
	}
	contents, _ := os.ReadFile(path)
	if strings.Contains(string(contents), token) {
		t.Fatal("plaintext token persisted")
	}
	if second, err := OpenStore(path); err == nil {
		second.Close()
		t.Fatal("concurrent writer accepted")
	}
	s.Close()
	s, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.now = func() time.Time { return now }
	now = now.Add(31 * time.Minute)
	if _, err = s.Get(token); err != nil {
		t.Fatal("logged out after half an hour", err)
	}
	now = now.Add(300 * 24 * time.Hour)
	renewed, err := s.Get(token)
	if err != nil {
		t.Fatal(err)
	}
	if !renewed.Expires.After(first.Expires) {
		t.Fatal("session did not renew")
	}
	now = first.Expires.Add(24 * time.Hour)
	if _, err = s.Get(token); err != nil {
		t.Fatal("active session did not survive original expiry", err)
	}
	if err = s.Delete(token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(token); err != ErrUnauthenticated {
		t.Fatal("logout did not revoke token")
	}
	_, _, err = s.Create(User{ID: "456"}, "")
	if err != nil {
		t.Fatal(err)
	}
}
func TestSessionExpiryAndFailedWrite(t *testing.T) {
	s, err := OpenStore(filepath.Join(t.TempDir(), "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	s.now = func() time.Time { return now }
	token, _, _ := s.Create(User{ID: "123"}, "")
	now = now.Add(SessionLifetime + time.Second)
	if _, err = s.Get(token); err != ErrUnauthenticated {
		t.Fatal("expired session accepted")
	}
	s.path = filepath.Join(t.TempDir(), "missing", "sessions.json")
	if token, _, err = s.Create(User{ID: "456"}, ""); err == nil || token != "" {
		t.Fatal("session issued after failed persistence")
	}
}
func fixture(t *testing.T) (*Auth, http.Handler) {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a := New(Config{ClientID: "test-client", Origins: []string{"https://club.example"}, SecureCookies: true}, s,
		func(context.Context, string, string, string) (User, error) {
			return User{ID: "123", Name: "Test", Email: "test@example.com"}, nil
		})
	mux := http.NewServeMux()
	a.Register(mux)
	return a, mux
}
func request(h http.Handler, method, path, origin, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "https://club.example"+path, strings.NewReader(body))
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func loginCookie(t *testing.T, a *Auth, h http.Handler) *http.Cookie {
	t.Helper()
	cfg := request(h, "GET", "/api/auth/config", "", "")
	login := request(h, "POST", "/api/auth/google", "https://club.example", `{"credential":"test"}`, cfg.Result().Cookies()...)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	for _, c := range login.Result().Cookies() {
		if c.Name == a.cookieName("session") {
			return c
		}
	}
	t.Fatal("missing session")
	return nil
}
func TestLoginCookieCSRFReplayAndLogout(t *testing.T) {
	a, h := fixture(t)
	if w := request(h, "GET", "/api/account", "", ""); w.Code != 401 {
		t.Fatal("anonymous account access")
	}
	cfg := request(h, "GET", "/api/auth/config", "", "")
	cookies := cfg.Result().Cookies()
	if w := request(h, "POST", "/api/auth/google", "https://evil.example", `{"credential":"test"}`, cookies...); w.Code != 403 {
		t.Fatal("cross-origin login accepted")
	}
	if w := request(h, "POST", "/api/auth/google", "", `{"credential":"test"}`, cookies...); w.Code != 403 {
		t.Fatal("originless login accepted")
	}
	first := request(h, "POST", "/api/auth/google", "https://club.example", `{"credential":"test"}`, cookies...)
	if first.Code != 200 {
		t.Fatal(first.Body.String())
	}
	if w := request(h, "POST", "/api/auth/google", "https://club.example", `{"credential":"test"}`, cookies...); w.Code != 401 {
		t.Fatal("nonce replay accepted")
	}
	c := loginCookie(t, a, h)
	if !c.HttpOnly || !c.Secure || c.Domain != "" || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.MaxAge < 364*86400 {
		t.Fatal("insecure or nonpersistent cookie", c.Name)
	}
	if w := request(h, "GET", "/api/account", "", "", c); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request(h, "POST", "/api/auth/logout", "https://evil.example", "", c); w.Code != 403 {
		t.Fatal("logout CSRF")
	}
	if w := request(h, "POST", "/api/auth/logout", "https://club.example", "", c); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := request(h, "GET", "/api/account", "", "", c); w.Code != 401 {
		t.Fatal("revoked cookie accepted")
	}
}
func TestAllowlistAndDisabledLogin(t *testing.T) {
	a, h := fixture(t)
	c := loginCookie(t, a, h)
	a.Config.AllowedEmails = []string{"different@example.com"}
	if w := request(h, "GET", "/api/account", "", "", c); w.Code != 401 {
		t.Fatal("allowlist bypass")
	}
	cfg := request(h, "GET", "/api/auth/config", "", "")
	if w := request(h, "POST", "/api/auth/google", "https://club.example", `{"credential":"test"}`, cfg.Result().Cookies()...); w.Code != 403 {
		t.Fatal("disallowed login")
	}
	a.Config.ClientID = ""
	if w := request(h, "POST", "/api/auth/google", "https://club.example", `{"credential":"test"}`); w.Code != 503 {
		t.Fatal("unconfigured login")
	}
	if err := ValidateConfig(Config{Origins: []string{"https://club.example"}}); err == nil {
		t.Fatal("insecure public cookies allowed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGoogleVerifierChecksRealSignatureAndClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := `{"keys":[{"kty":"RSA","use":"sig","alg":"RS256","kid":"test","n":"` + base64.RawURLEncoding.EncodeToString(key.N.Bytes()) + `","e":"AQAB"}]}`
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Cache-Control": []string{"public, max-age=3600"}}, Body: io.NopCloser(strings.NewReader(jwks))}, nil
	})}
	verify, err := googleVerifier(client)
	if err != nil {
		t.Fatal(err)
	}
	token := func(change func(map[string]any)) string {
		claims := map[string]any{"iss": "https://accounts.google.com", "aud": "test-client", "sub": "123", "email": "test@example.com", "email_verified": true, "name": "Test", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "nonce": "test-nonce"}
		if change != nil {
			change(claims)
		}
		body, _ := json.Marshal(claims)
		encoded := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test"}`)) + "." + base64.RawURLEncoding.EncodeToString(body)
		digest := sha256.Sum256([]byte(encoded))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return encoded + "." + base64.RawURLEncoding.EncodeToString(signature)
	}
	if user, err := verify(context.Background(), token(nil), "test-client", "test-nonce"); err != nil || user.ID != "123" {
		t.Fatal("valid signed token rejected", err)
	}
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"audience", func(c map[string]any) { c["aud"] = "other-client" }},
		{"issuer", func(c map[string]any) { c["iss"] = "https://evil.example" }},
		{"expiry", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{"email", func(c map[string]any) { c["email_verified"] = false }},
		{"nonce", func(c map[string]any) { c["nonce"] = "different" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := verify(context.Background(), token(test.change), "test-client", "test-nonce"); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
	parts := strings.Split(token(nil), ".")
	parts[2] = base64.RawURLEncoding.EncodeToString(make([]byte, 256))
	if _, err := verify(context.Background(), strings.Join(parts, "."), "test-client", "test-nonce"); err == nil {
		t.Fatal("forged signature accepted")
	}
}
