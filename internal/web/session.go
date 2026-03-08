package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/raphi011/knowhow/internal/models"
)

// Session holds per-user state. Fields are protected by mu for concurrent access.
type Session struct {
	mu            sync.Mutex
	ID            string
	UserID        string
	VaultPermissions []models.VaultPermission
	SelectedVault string
	Locale        string
	Theme         string
	CreatedAt     time.Time
	LastAccess    time.Time
}

// SetLocale updates the locale with proper synchronization.
func (s *Session) SetLocale(locale string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Locale = locale
}

// SetTheme updates the theme with proper synchronization.
func (s *Session) SetTheme(theme string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Theme = theme
}

// SetSelectedVault updates the selected vault with proper synchronization.
func (s *Session) SetSelectedVault(vaultID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SelectedVault = vaultID
}

// Snapshot returns a copy of the mutable session fields for safe concurrent reads.
func (s *Session) Snapshot() (locale, theme, selectedVault string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Locale, s.Theme, s.SelectedVault
}

// SessionStore is an in-memory session store backed by sync.Map.
type SessionStore struct {
	sessions sync.Map
	ttl      time.Duration
	done     chan struct{}
}

// NewSessionStore creates a session store with the given TTL.
func NewSessionStore(ttl time.Duration) *SessionStore {
	s := &SessionStore{ttl: ttl, done: make(chan struct{})}
	go s.reapLoop()
	return s
}

// Close stops the session reaper goroutine.
func (s *SessionStore) Close() {
	close(s.done)
}

const sessionCookieName = "kh_sid"

// Create generates a new session and returns it.
func (s *SessionStore) Create(userID string, vaultPerms []models.VaultPermission, selectedVault string) *Session {
	id := generateSessionID()
	sess := &Session{
		ID:               id,
		UserID:           userID,
		VaultPermissions: vaultPerms,
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
	for {
		select {
		case <-ticker.C:
			s.sessions.Range(func(key, value any) bool {
				sess := value.(*Session)
				if time.Since(sess.LastAccess) > s.ttl {
					s.sessions.Delete(key)
				}
				return true
			})
		case <-s.done:
			return
		}
	}
}

func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate session ID: " + err.Error())
	}
	return hex.EncodeToString(b)
}
