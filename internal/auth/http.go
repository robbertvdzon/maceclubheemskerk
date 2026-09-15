package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ClientID      string
	Origins       []string
	AllowedEmails []string
	SecureCookies bool
}

func ValidateConfig(cfg Config) error {
	for _, origin := range cfg.Origins {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return errors.New("invalid APP_ORIGINS")
		}
		local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
		if u.Scheme != "https" && !(u.Scheme == "http" && local) {
			return errors.New("HTTPS is required")
		}
		if !cfg.SecureCookies && !local {
			return errors.New("insecure cookies are only allowed on localhost")
		}
	}
	return nil
}

type Auth struct {
	Config     Config
	Store      *Store
	verify     VerifyFunc
	mu         sync.Mutex
	challenges map[string]time.Time
}

func New(cfg Config, store *Store, verify VerifyFunc) *Auth {
	return &Auth{Config: cfg, Store: store, verify: verify, challenges: map[string]time.Time{}}
}
func (a *Auth) Enabled() bool { return a.Config.ClientID != "" && a.Store != nil && a.verify != nil }
func (a *Auth) cookieName(suffix string) string {
	if a.Config.SecureCookies {
		return "__Host-mch-" + suffix
	}
	return "mch-local-" + suffix
}
func (a *Auth) cookie(w http.ResponseWriter, suffix, value string, expires time.Time, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(suffix), Value: value, Path: "/", HttpOnly: true, Secure: a.Config.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: maxAge})
}
func (a *Auth) token(r *http.Request, suffix string) string {
	c, e := r.Cookie(a.cookieName(suffix))
	if e != nil {
		return ""
	}
	return c.Value
}
func output(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	output(w, status, map[string]string{"error": message})
}

// Mutations must originate from this exact origin. No forwarded headers or wildcard CORS.
func (a *Auth) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	for _, allowed := range a.Config.Origins {
		if origin == allowed {
			u, e := url.Parse(origin)
			return e == nil && u.Host == r.Host
		}
	}
	return false
}
func (a *Auth) allowed(user User) bool {
	if len(a.Config.AllowedEmails) == 0 {
		return true
	}
	for _, email := range a.Config.AllowedEmails {
		if strings.EqualFold(email, user.Email) {
			return true
		}
	}
	return false
}
func (a *Auth) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/config", a.configuration)
	mux.HandleFunc("POST /api/auth/google", a.login)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("GET /api/auth/me", a.me)
	// This endpoint is the protected account surface, and a pattern for future member APIs.
	mux.HandleFunc("GET /api/account", a.RequireUser(func(w http.ResponseWriter, r *http.Request, user User) { output(w, 200, map[string]any{"user": user}) }))
}
func (a *Auth) configuration(w http.ResponseWriter, r *http.Request) {
	if !a.Enabled() {
		output(w, 200, map[string]any{"enabled": false})
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for token, expiry := range a.challenges {
		if !now.Before(expiry) {
			delete(a.challenges, token)
		}
	}
	if len(a.challenges) >= 2000 {
		fail(w, 429, "Probeer het over enkele minuten opnieuw.")
		return
	}
	nonce := randomToken()
	a.challenges[hash(nonce)] = now.Add(10 * time.Minute)
	a.cookie(w, "nonce", nonce, now.Add(10*time.Minute), 600)
	output(w, 200, map[string]any{"enabled": true, "clientId": a.Config.ClientID, "nonce": nonce})
}
func (a *Auth) login(w http.ResponseWriter, r *http.Request) {
	if !a.sameOrigin(r) {
		fail(w, 403, "Ongeldige herkomst.")
		return
	}
	if !a.Enabled() {
		fail(w, 503, "Inloggen is nog niet beschikbaar.")
		return
	}
	var body struct {
		Credential string `json:"credential"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Credential == "" {
		fail(w, 400, "Ongeldige aanvraag.")
		return
	}
	nonce := a.token(r, "nonce")
	a.mu.Lock()
	expiry, ok := a.challenges[hash(nonce)]
	delete(a.challenges, hash(nonce))
	a.mu.Unlock()
	a.cookie(w, "nonce", "", time.Unix(1, 0), -1)
	if !ok || !time.Now().Before(expiry) {
		fail(w, 401, "Start het inloggen opnieuw.")
		return
	}
	user, err := a.verify(r.Context(), body.Credential, a.Config.ClientID, nonce)
	if err != nil {
		fail(w, 401, "Google-login kon niet worden gecontroleerd. Probeer opnieuw.")
		return
	}
	if !a.allowed(user) {
		fail(w, 403, "Dit account heeft geen toegang.")
		return
	}
	token, session, err := a.Store.Create(user, a.token(r, "session"))
	if err != nil {
		fail(w, 503, "Inloggen is tijdelijk niet beschikbaar.")
		return
	}
	a.setSession(w, token, session)
	output(w, 200, map[string]any{"user": user})
}
func (a *Auth) setSession(w http.ResponseWriter, token string, s Session) {
	a.cookie(w, "session", token, s.Expires, int(time.Until(s.Expires).Seconds()))
}
func (a *Auth) current(w http.ResponseWriter, r *http.Request) (User, error) {
	if !a.Enabled() {
		return User{}, ErrUnauthenticated
	}
	token := a.token(r, "session")
	session, err := a.Store.Get(token)
	if err != nil {
		return User{}, err
	}
	if !a.allowed(session.User) {
		return User{}, ErrUnauthenticated
	}
	a.setSession(w, token, session)
	return session.User, nil
}
func (a *Auth) me(w http.ResponseWriter, r *http.Request) {
	u, err := a.current(w, r)
	if errors.Is(err, ErrUnauthenticated) {
		output(w, 200, map[string]any{"user": nil})
		return
	}
	if err != nil {
		fail(w, 503, "Je account is tijdelijk niet bereikbaar.")
		return
	}
	output(w, 200, map[string]any{"user": u})
}
func (a *Auth) RequireUser(next func(http.ResponseWriter, *http.Request, User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" && !a.sameOrigin(r) {
			fail(w, 403, "Ongeldige herkomst.")
			return
		}
		u, err := a.current(w, r)
		if errors.Is(err, ErrUnauthenticated) {
			fail(w, 401, "Log in om deze functie te gebruiken.")
			return
		}
		if err != nil {
			fail(w, 503, "Je account is tijdelijk niet bereikbaar.")
			return
		}
		next(w, r, u)
	}
}
func (a *Auth) logout(w http.ResponseWriter, r *http.Request) {
	if !a.sameOrigin(r) {
		fail(w, 403, "Ongeldige herkomst.")
		return
	}
	if a.Store != nil {
		if err := a.Store.Delete(a.token(r, "session")); err != nil {
			fail(w, 503, "Uitloggen is niet gelukt. Probeer opnieuw.")
			return
		}
	}
	a.cookie(w, "session", "", time.Unix(1, 0), -1)
	output(w, 200, map[string]bool{"ok": true})
}
