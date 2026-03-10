package integration

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/mcptools"
	"github.com/raphi011/knowhow/internal/parser"
	"github.com/raphi011/knowhow/internal/search"
	"github.com/raphi011/knowhow/internal/tools"
	"github.com/raphi011/knowhow/internal/vault"
)

// setupMCPServer creates a vault, executor, MCP handler, and httptest.Server.
// Auth is bypassed via NoAuthMiddleware.
func setupMCPServer(t *testing.T, suffix string) (*httptest.Server, string, *document.Service) {
	t.Helper()
	ctx := context.Background()
	vaultID, _ := setupVault(t, ctx, "mcp-"+suffix+"-"+fmt.Sprint(time.Now().UnixNano()))

	searchSvc := search.NewService(testDB, nil)
	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)
	vaultSvc := vault.NewService(testDB)
	executor := &tools.Executor{
		DB:         testDB,
		Search:     searchSvc,
		DocService: docSvc,
	}

	handler := mcptools.NewHandler(executor, testDB, vaultSvc)
	wrappedHandler := auth.NoAuthMiddleware(handler)

	srv := httptest.NewServer(wrappedHandler)
	t.Cleanup(srv.Close)

	return srv, vaultID, docSvc
}

// connectMCPClient creates an MCP client session connected to the given server.
func connectMCPClient(t *testing.T, ctx context.Context, srv *httptest.Server) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "0.0.1",
	}, nil)

	transport := &mcp.StreamableClientTransport{
		Endpoint:   srv.URL,
		HTTPClient: srv.Client(),
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Logf("close MCP session: %v", err)
		}
	})

	return session
}

func TestMCP_ListTools(t *testing.T) {
	srv, _, _ := setupMCPServer(t, "list-tools")
	ctx := context.Background()
	session := connectMCPClient(t, ctx, srv)

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expectedTools := map[string]bool{
		"search_documents":     false,
		"get_document":         false,
		"list_labels":          false,
		"list_folders":         false,
		"list_folder_contents": false,
		"get_document_versions": false,
		"create_memory":        false,
		"create_document":      false,
		"edit_document":        false,
		"edit_document_section": false,
	}

	for _, tool := range result.Tools {
		if _, ok := expectedTools[tool.Name]; ok {
			expectedTools[tool.Name] = true
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("missing expected tool: %s", name)
		}
	}
}

func TestMCP_CreateAndGetDocument(t *testing.T) {
	srv, _, _ := setupMCPServer(t, "create-get")
	ctx := context.Background()
	session := connectMCPClient(t, ctx, srv)

	// Create
	createResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_document",
		Arguments: map[string]any{
			"path":    "/mcp-test/doc.md",
			"content": "# MCP Test\n\nCreated via MCP protocol.",
		},
	})
	if err != nil {
		t.Fatalf("CallTool create_document: %v", err)
	}
	if createResult.IsError {
		t.Fatalf("create_document returned error: %s", contentText(createResult))
	}
	if !strings.Contains(contentText(createResult), "Document created") {
		t.Errorf("unexpected create result: %s", contentText(createResult))
	}

	// Get
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_document",
		Arguments: map[string]any{
			"path": "/mcp-test/doc.md",
		},
	})
	if err != nil {
		t.Fatalf("CallTool get_document: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("get_document returned error: %s", contentText(getResult))
	}
	text := contentText(getResult)
	if !strings.Contains(text, "MCP Test") {
		t.Errorf("get_document missing title: %s", text)
	}
	if !strings.Contains(text, "Created via MCP protocol.") {
		t.Errorf("get_document missing content: %s", text)
	}
}

func TestMCP_SearchDocuments(t *testing.T) {
	srv, _, docSvc := setupMCPServer(t, "search")
	ctx := context.Background()
	session := connectMCPClient(t, ctx, srv)

	// Create a document to search for
	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_document",
		Arguments: map[string]any{
			"path":    "/mcp-search/kubernetes.md",
			"content": "# Kubernetes\n\nContainer orchestration platform for managing microservices.",
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := docSvc.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_documents",
		Arguments: map[string]any{
			"query": "kubernetes",
		},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.IsError {
		t.Fatalf("search returned error: %s", contentText(result))
	}
	if !strings.Contains(contentText(result), "Kubernetes") {
		t.Errorf("search result missing expected doc: %s", contentText(result))
	}
}

func TestMCP_EditDocumentSection(t *testing.T) {
	srv, _, _ := setupMCPServer(t, "edit-section")
	ctx := context.Background()
	session := connectMCPClient(t, ctx, srv)

	// Create multi-section doc
	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_document",
		Arguments: map[string]any{
			"path":    "/mcp-section/doc.md",
			"content": "# Doc\n\nIntro.\n\n## Alpha\n\nAlpha content.\n\n## Beta\n\nBeta content.",
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Edit section Alpha
	editResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edit_document_section",
		Arguments: map[string]any{
			"path":      "/mcp-section/doc.md",
			"operation": "replace",
			"heading":   "Alpha",
			"content":   "New alpha content via MCP.",
		},
	})
	if err != nil {
		t.Fatalf("edit_document_section: %v", err)
	}
	if editResult.IsError {
		t.Fatalf("edit returned error: %s", contentText(editResult))
	}

	// Verify
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_document",
		Arguments: map[string]any{
			"path": "/mcp-section/doc.md",
		},
	})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	text := contentText(getResult)
	if !strings.Contains(text, "New alpha content via MCP.") {
		t.Errorf("section not updated: %s", text)
	}
	if !strings.Contains(text, "Beta content.") {
		t.Errorf("other section should be intact: %s", text)
	}
}

func TestMCP_ErrorHandling(t *testing.T) {
	srv, _, _ := setupMCPServer(t, "errors")
	ctx := context.Background()
	session := connectMCPClient(t, ctx, srv)

	// Call create_document with empty path
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_document",
		Arguments: map[string]any{
			"path":    "",
			"content": "# Test",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty path")
	}

	// Call create_document with empty content
	result, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_document",
		Arguments: map[string]any{
			"path":    "/valid.md",
			"content": "",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty content")
	}

	// Call edit_document on nonexistent doc — should return IsError
	result, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edit_document",
		Arguments: map[string]any{
			"path":    "/does-not-exist.md",
			"content": "# Ghost",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for editing nonexistent doc")
	}
}

// contentText extracts the text from a CallToolResult.
func contentText(result *mcp.CallToolResult) string {
	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

