// Package logutil provides context-carried slog loggers for structured request-scoped logging.
package logutil

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

// WithLogger stores an *slog.Logger in the context.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromCtx extracts the logger from ctx, falling back to slog.Default().
func FromCtx(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
