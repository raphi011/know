package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionMiddleware_ExpiredSession(t *testing.T) {
	store := NewSessionStore(1 * time.Millisecond)
	sess := store.Create("user1", []string{"vault:default"}, "vault:default")

	time.Sleep(5 * time.Millisecond)

	handler := SessionMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called with expired session")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func TestSessionMiddleware_InvalidCookie(t *testing.T) {
	store := NewSessionStore(1 * time.Hour)

	handler := SessionMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called with invalid session")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "nonexistent-id"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func TestSessionFromContext_NoSession_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when calling SessionFromContext without middleware")
		}
	}()
	req := httptest.NewRequest("GET", "/", nil)
	SessionFromContext(req.Context())
}

func TestHasVaultAccess(t *testing.T) {
	sess := &Session{VaultAccess: []string{"vault:a", "vault:b"}}

	if !hasVaultAccess(sess, "vault:a") {
		t.Error("expected access to vault:a")
	}
	if !hasVaultAccess(sess, "vault:b") {
		t.Error("expected access to vault:b")
	}
	if hasVaultAccess(sess, "vault:c") {
		t.Error("expected no access to vault:c")
	}
}
