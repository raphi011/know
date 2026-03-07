package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFMiddleware_AllowsGET(t *testing.T) {
	called := false
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected GET to pass through")
	}
}

func TestCSRFMiddleware_BlocksPOSTWithoutOriginOrReferer(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("POST", "/hx/vault/switch", nil)
	req.Host = "localhost:8484"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFMiddleware_AllowsPOSTWithMatchingOrigin(t *testing.T) {
	called := false
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("POST", "/hx/vault/switch", nil)
	req.Host = "localhost:8484"
	req.Header.Set("Origin", "http://localhost:8484")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected POST with matching Origin to pass through")
	}
}

func TestCSRFMiddleware_BlocksPOSTWithMismatchedOrigin(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("POST", "/hx/vault/switch", nil)
	req.Host = "localhost:8484"
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestCSRFMiddleware_AllowsPOSTWithMatchingReferer(t *testing.T) {
	called := false
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("POST", "/hx/vault/switch", nil)
	req.Host = "localhost:8484"
	req.Header.Set("Referer", "http://localhost:8484/settings")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected POST with matching Referer to pass through")
	}
}

func TestCSRFMiddleware_BlocksPOSTWithMismatchedReferer(t *testing.T) {
	handler := CSRFMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest("POST", "/hx/vault/switch", nil)
	req.Host = "localhost:8484"
	req.Header.Set("Referer", "http://evil.example.com/attack")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
