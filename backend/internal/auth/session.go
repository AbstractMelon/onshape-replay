// Package auth handles per-user OAuth2 sessions with Onshape.
// Sessions are stored in memory keyed by a random session ID stored in a
// cookie. Cookies use SameSite=None + Secure so they work inside iframes.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const (
	cookieName = "oreplay_session"
	// Sessions live for 30 days. The refresh token keeps the Onshape access
	// alive; if it expires the user must re-authorize.
	sessionTTL = 30 * 24 * time.Hour
)

// Session holds per-user data stored server-side.
type Session struct {
	ID           string
	AccessToken  string
	RefreshToken string
	CreatedAt    time.Time
	// State is a random nonce used during the OAuth round-trip to prevent CSRF.
	OAuthState string
	// RedirectAfterAuth stores the original panel URL so we can bounce back
	// after the OAuth callback.
	RedirectAfterAuth string
}

// Store is an in-memory session store.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewStore returns an initialized session store.
func NewStore() *Store {
	return &Store{sessions: make(map[string]*Session)}
}

// Get retrieves the session associated with the request cookie.
// Returns nil, false if no valid session exists.
func (s *Store) Get(r *http.Request) (*Session, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[c.Value]
	if !ok {
		return nil, false
	}
	if time.Since(sess.CreatedAt) > sessionTTL {
		return nil, false
	}
	return sess, true
}

// Create creates a new session and sets the cookie on the response.
func (s *Store) Create(w http.ResponseWriter) (*Session, error) {
	id, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	sess := &Session{
		ID:        id,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	s.setCookie(w, id)
	return sess, nil
}

// Save persists changes to an existing session. Must be called after mutating
// a session returned by Get.
func (s *Store) Save(w http.ResponseWriter, sess *Session) {
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	s.setCookie(w, sess.ID)
}

// Delete removes the session and clears the cookie.
func (s *Store) Delete(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
	})
}

func (s *Store) setCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		MaxAge:   int(sessionTTL.Seconds()),
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteNoneMode,
	})
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
