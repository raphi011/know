// Package mcptools provides an embedded MCP server handler that exposes
// know tools over the Model Context Protocol.
package mcptools

import (
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphi011/know/internal/apify"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/jina"
	"github.com/raphi011/know/internal/memory"
	"github.com/raphi011/know/internal/remote"
	"github.com/raphi011/know/internal/tools"
	"github.com/raphi011/know/internal/vault"
)

// NewHandler creates the MCP HTTP handler that serves know tools at the
// given path. Auth is handled externally via auth.Middleware wrapping this handler.
// remoteService and memoryService may be nil if not configured.
func NewHandler(executor tools.ToolExecutor, dbClient *db.Client, vaultService *vault.Service, remoteService *remote.Service, memoryService *memory.Service, apifyClient *apify.Client, jinaClient *jina.Client) http.Handler {
	t := &mcpTools{
		executor:      executor,
		db:            dbClient,
		vaultService:  vaultService,
		remoteService: remoteService,
		memoryService: memoryService,
		apifyClient:   apifyClient,
		jinaClient:    jinaClient,
		cache:         newCache(60 * time.Second),
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "know",
		Version: "0.1.0",
	}, nil)

	t.register(server)

	return mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return server },
		nil,
	)
}
