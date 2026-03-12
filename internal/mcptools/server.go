// Package mcptools provides an embedded MCP server handler that exposes
// knowhow tools over the Model Context Protocol.
package mcptools

import (
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/memory"
	"github.com/raphi011/knowhow/internal/remote"
	"github.com/raphi011/knowhow/internal/tools"
	"github.com/raphi011/knowhow/internal/vault"
)

// NewHandler creates the MCP HTTP handler that serves knowhow tools at the
// given path. Auth is handled externally via auth.Middleware wrapping this handler.
// remoteService and memoryService may be nil if not configured.
func NewHandler(executor tools.ToolExecutor, dbClient *db.Client, vaultService *vault.Service, remoteService *remote.Service, memoryService *memory.Service) http.Handler {
	t := &mcpTools{
		executor:      executor,
		db:            dbClient,
		vaultService:  vaultService,
		remoteService: remoteService,
		memoryService: memoryService,
		cache:         newCache(60 * time.Second),
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "knowhow",
		Version: "0.1.0",
	}, nil)

	t.register(server)

	return mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return server },
		nil,
	)
}
