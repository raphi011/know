package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/raphi011/know/internal/logutil"
)

// AuditEvent represents the type of auth event being logged.
type AuditEvent string

const (
	AuditSuccess AuditEvent = "auth.success"
	AuditFailure AuditEvent = "auth.failure"
	AuditExpired AuditEvent = "auth.expired"
)

// AuditResult represents the outcome of an auth event.
type AuditResult string

const (
	AuditResultOK      AuditResult = "ok"
	AuditResultDenied  AuditResult = "denied"
	AuditResultExpired AuditResult = "expired"
)

// Result returns the canonical AuditResult for the event, ensuring
// event and result are always paired consistently.
func (e AuditEvent) Result() AuditResult {
	switch e {
	case AuditSuccess:
		return AuditResultOK
	case AuditExpired:
		return AuditResultExpired
	default:
		return AuditResultDenied
	}
}

// AuditLog logs a structured auth event for the audit trail.
// Uses the context logger so request-scoped fields (request_id, user_id) propagate.
func AuditLog(ctx context.Context, event AuditEvent, attrs ...slog.Attr) {
	result := event.Result()
	args := make([]any, 0, len(attrs)+2)
	args = append(args, slog.String("event", string(event)))
	args = append(args, slog.String("result", string(result)))
	for _, a := range attrs {
		args = append(args, a)
	}
	logutil.FromCtx(ctx).Info("audit", args...)
}

// clientIP extracts the client IP from an HTTP request,
// preferring the first entry in X-Forwarded-For (for reverse proxy setups)
// over RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// XFF is "client, proxy1, proxy2" — take the first (client) IP.
		if before, _, ok := strings.Cut(xff, ","); ok {
			return strings.TrimSpace(before)
		}
		return xff
	}
	return r.RemoteAddr
}
