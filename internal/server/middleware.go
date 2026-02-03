package server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxArgLogLen is the maximum length for logged arguments before truncation.
const maxArgLogLen = 200

// slowRequestThreshold is the duration above which requests are logged at WARN level.
const slowRequestThreshold = 100 * time.Millisecond

// LoggingMiddleware returns middleware that logs all requests with timing.
// Slow requests (>100ms) are logged at WARN level.
// Arguments are truncated to 200 characters.
func LoggingMiddleware(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			start := time.Now()

			// Call the next handler
			result, err := next(ctx, method, req)

			duration := time.Since(start)

			// Build log attributes
			attrs := []any{
				"method", method,
				"duration_ms", duration.Milliseconds(),
			}

			// Add truncated params if present
			if params := formatParams(req); params != "" {
				attrs = append(attrs, "params", truncate(params, maxArgLogLen))
			}

			// Log based on duration and error
			if err != nil {
				attrs = append(attrs, "error", err.Error())
				logger.Error("request failed", attrs...)
			} else if duration > slowRequestThreshold {
				logger.Warn("slow request", attrs...)
			} else {
				logger.Debug("request completed", attrs...)
			}

			return result, err
		}
	}
}

// formatParams extracts and formats request parameters for logging.
func formatParams(req mcp.Request) string {
	// Try to get params using the Request interface
	params := req.GetParams()
	if params == nil {
		return ""
	}
	return fmt.Sprintf("%+v", params)
}

// truncate shortens a string to maxLen, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
