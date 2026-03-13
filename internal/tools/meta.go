package tools

import (
	"context"
	"log/slog"
)

type resultMetaKey struct{}

// WithResultMeta returns a context that can carry a ToolResultMeta value set
// by InvokableTool implementations. Use ResultMeta to retrieve it after
// InvokableRun returns.
func WithResultMeta(ctx context.Context) context.Context {
	return context.WithValue(ctx, resultMetaKey{}, &metaHolder{})
}

// ResultMeta returns the ToolResultMeta stored in the context by a tool's
// InvokableRun, or nil if none was set.
func ResultMeta(ctx context.Context) *ToolResultMeta {
	h, ok := ctx.Value(resultMetaKey{}).(*metaHolder)
	if !ok || h == nil {
		return nil
	}
	return h.meta
}

// setResultMeta stores a ToolResultMeta in the context. Called by tool
// implementations within InvokableRun. Callers must prepare the context
// with WithResultMeta first; logs a warning if they didn't.
func setResultMeta(ctx context.Context, meta *ToolResultMeta) {
	h, ok := ctx.Value(resultMetaKey{}).(*metaHolder)
	if !ok || h == nil {
		slog.Warn("setResultMeta called without WithResultMeta context — metadata discarded")
		return
	}
	h.meta = meta
}

type metaHolder struct {
	meta *ToolResultMeta
}
