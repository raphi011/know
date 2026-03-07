package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// Session holds per-user state.
type Session struct {
	ID            string
	UserID        string
	VaultAccess   []string
	SelectedVault string
	Locale        string
	Theme         string
	CreatedAt     time.Time
	LastAccess    time.Time
}

// SessionStore is an in-memory session store backed by sync.Map.
type SessionStore struct {
	sessions sync.Map
	ttl      time.Duration
}

// NewSessionStore creates a session store with the given TTL.
func NewSessionStore(ttl time.Duration) *SessionStore {
	s := &SessionStore{ttl: ttl}
	go s.reapLoop()
	return s
}

const sessionCookieName = "kh_sid"

// Create generates a new session and returns it.
func (s *SessionStore) Create(userID string, vaultAccess []string, selectedVault string) *Session {
	id := generateSessionID()
	sess := &Session{
		ID:            id,
		UserID:        userID,
		VaultAccess:   vaultAccess,
		SelectedVault: selectedVault,
		Locale:        "en",
		Theme:         "system",
		CreatedAt:     time.Now(),
		LastAccess:    time.Now(),
	}
	s.sessions.Store(id, sess)
	return sess
}

// Get retrieves a session by ID, returning nil if not found or expired.
func (s *SessionStore) Get(id string) *Session {
	v, ok := s.sessions.Load(id)
	if !ok {
		return nil
	}
	sess := v.(*Session)
	if time.Since(sess.LastAccess) > s.ttl {
		s.sessions.Delete(id)
		return nil
	}
	sess.LastAccess = time.Now()
	return sess
}

// Delete removes a session.
func (s *SessionStore) Delete(id string) {
	s.sessions.Delete(id)
}

// SetCookie writes the session cookie to the response.
func (s *SessionStore) SetCookie(w http.ResponseWriter, sess *Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.ttl.Seconds()),
	})
}

// ClearCookie removes the session cookie.
func (s *SessionStore) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// FromRequest extracts the session from a request cookie.
func (s *SessionStore) FromRequest(r *http.Request) *Session {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil
	}
	return s.Get(c.Value)
}

// reapLoop periodically removes expired sessions.
func (s *SessionStore) reapLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.sessions.Range(func(key, value any) bool {
			sess := value.(*Session)
			if time.Since(sess.LastAccess) > s.ttl {
				s.sessions.Delete(key)
			}
			return true
		})
	}
}

func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate session ID: " + err.Error())
	}
	return hex.EncodeToString(b)
}
