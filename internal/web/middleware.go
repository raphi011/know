package web

import (
	"context"
	"net/http"
)

type sessionContextKey struct{}

// SessionMiddleware checks for a valid session cookie and injects it into context.
// Redirects to /login if no valid session found.
func SessionMiddleware(store *SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess := store.FromRequest(r)
			if sess == nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			ctx := context.WithValue(r.Context(), sessionContextKey{}, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SessionFromContext retrieves the session from context. Panics if called
// without SessionMiddleware — this is a programming error, not a runtime condition.
func SessionFromContext(ctx context.Context) *Session {
	sess, ok := ctx.Value(sessionContextKey{}).(*Session)
	if !ok || sess == nil {
		panic("web: SessionFromContext called without SessionMiddleware")
	}
	return sess
}
