// Package tools provides MCP tool handlers and registration.
package tools

import (
	"log/slog"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/embedding"
)

// Dependencies holds shared services for tool handlers.
// Passed to handler factories via closure capture.
type Dependencies struct {
	DB       *db.Client
	Embedder embedding.Embedder
	Logger   *slog.Logger
}
