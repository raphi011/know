package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionStore_CreateAndGet(t *testing.T) {
	store := NewSessionStore(time.Hour)

	sess := store.Create("user1", []string{"vault:default"}, "default")
	if sess.ID == "" {
		t.Fatal("session ID should not be empty")
	}
	if sess.UserID != "user1" {
		t.Errorf("UserID = %q, want %q", sess.UserID, "user1")
	}

	got := store.Get(sess.ID)
	if got == nil {
		t.Fatal("Get() returned nil for valid session")
	}
	if got.ID != sess.ID {
		t.Errorf("ID = %q, want %q", got.ID, sess.ID)
	}
}

func TestSessionStore_GetExpired(t *testing.T) {
	store := NewSessionStore(1 * time.Millisecond)

	sess := store.Create("user1", []string{"vault:default"}, "default")
	time.Sleep(5 * time.Millisecond)

	got := store.Get(sess.ID)
	if got != nil {
		t.Error("Get() should return nil for expired session")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	store := NewSessionStore(time.Hour)

	sess := store.Create("user1", []string{"vault:default"}, "default")
	store.Delete(sess.ID)

	got := store.Get(sess.ID)
	if got != nil {
		t.Error("Get() should return nil after Delete()")
	}
}

func TestSessionStore_GetNotFound(t *testing.T) {
	store := NewSessionStore(time.Hour)

	got := store.Get("nonexistent")
	if got != nil {
		t.Error("Get() should return nil for nonexistent session")
	}
}

func TestSessionStore_Cookie(t *testing.T) {
	store := NewSessionStore(time.Hour)
	sess := store.Create("user1", []string{"vault:default"}, "default")

	// Set cookie
	rec := httptest.NewRecorder()
	store.SetCookie(rec, sess)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != sessionCookieName {
		t.Errorf("cookie name = %q, want %q", cookies[0].Name, sessionCookieName)
	}
	if cookies[0].Value != sess.ID {
		t.Errorf("cookie value = %q, want %q", cookies[0].Value, sess.ID)
	}
	if !cookies[0].HttpOnly {
		t.Error("cookie should be HttpOnly")
	}

	// FromRequest
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookies[0])

	got := store.FromRequest(req)
	if got == nil {
		t.Fatal("FromRequest() returned nil")
	}
	if got.ID != sess.ID {
		t.Errorf("FromRequest().ID = %q, want %q", got.ID, sess.ID)
	}
}

func TestSessionStore_ClearCookie(t *testing.T) {
	store := NewSessionStore(time.Hour)

	rec := httptest.NewRecorder()
	store.ClearCookie(rec)

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].MaxAge != -1 {
		t.Errorf("cookie MaxAge = %d, want -1", cookies[0].MaxAge)
	}
}

func TestSessionMiddleware_NoSession(t *testing.T) {
	store := NewSessionStore(time.Hour)
	mw := SessionMiddleware(store)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without session")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want %q", loc, "/login")
	}
}

func TestSessionMiddleware_ValidSession(t *testing.T) {
	store := NewSessionStore(time.Hour)
	sess := store.Create("user1", []string{"vault:default"}, "default")

	var called bool
	mw := SessionMiddleware(store)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		s := SessionFromContext(r.Context())
		if s == nil {
			t.Error("SessionFromContext() returned nil")
		} else if s.ID != sess.ID {
			t.Errorf("session ID = %q, want %q", s.ID, sess.ID)
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}
