package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const SessionLifetime = 365 * 24 * time.Hour

var ErrUnauthenticated = errors.New("unauthenticated")

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}
type Session struct {
	User    User      `json:"user"`
	Expires time.Time `json:"expires"`
	Touched time.Time `json:"touched"`
}

// Store persists only token hashes. One process owns the file; use Recreate on OpenShift.
type Store struct {
	mu       sync.Mutex
	path     string
	lock     *os.File
	sessions map[string]Session
	now      func() time.Time
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, err
	}
	s := &Store{path: path, lock: lock, sessions: map[string]Session{}, now: time.Now}
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &s.sessions)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.Close()
		return nil, err
	}
	if s.sessions == nil {
		s.Close()
		return nil, errors.New("invalid session store")
	}
	return s, nil
}

func (s *Store) Close() error { return s.lock.Close() }
func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func hash(token string) string { h := sha256.Sum256([]byte(token)); return hex.EncodeToString(h[:]) }

// Commit by rename before changing memory, so a failed write never issues a session.
func (s *Store) save(next map[string]Session) error {
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".sessions-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.sessions = next
	return nil
}
func (s *Store) active() map[string]Session {
	next := map[string]Session{}
	for k, v := range s.sessions {
		if s.now().Before(v.Expires) {
			next[k] = v
		}
	}
	return next
}
func (s *Store) Create(user User, previous string) (string, Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.active()
	delete(next, hash(previous))
	if len(next) >= 10000 {
		return "", Session{}, errors.New("session capacity reached")
	}
	token := randomToken()
	session := Session{User: user, Expires: s.now().Add(SessionLifetime), Touched: s.now()}
	next[hash(token)] = session
	if err := s.save(next); err != nil {
		return "", Session{}, err
	}
	return token, session, nil
}
func (s *Store) Get(token string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[hash(token)]
	if !ok || !s.now().Before(v.Expires) {
		return Session{}, ErrUnauthenticated
	}
	if s.now().Sub(v.Touched) >= time.Hour {
		v.Touched = s.now()
		v.Expires = s.now().Add(SessionLifetime)
		next := s.active()
		next[hash(token)] = v
		if err := s.save(next); err != nil {
			return Session{}, err
		}
	}
	return v, nil
}
func (s *Store) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.active()
	delete(next, hash(token))
	return s.save(next)
}
