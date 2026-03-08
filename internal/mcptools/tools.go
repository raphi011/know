package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/tools"
	"github.com/raphi011/knowhow/internal/vault"
)

type mcpTools struct {
	executor     *tools.Executor
	db           *db.Client
	vaultService *vault.Service
}

func (t *mcpTools) register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_documents",
		Description: "Search documents using full-text and semantic search. Returns titles, paths, scores, and matching snippets.",
	}, t.searchDocuments)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_document",
		Description: "Get a document by its path. Returns the full content, title, labels, and metadata. Set sections=true to include a section outline for use with edit_document_section.",
	}, t.getDocument)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_labels",
		Description: "List all labels used across documents. Useful for discovering available categories before searching.",
	}, t.listLabels)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_folders",
		Description: "List the folder structure. Use to browse and understand vault organization before searching.",
	}, t.listFolders)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_folder_contents",
		Description: "List documents and subfolders in a specific folder. Returns immediate children only.",
	}, t.listFolderContents)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_document_versions",
		Description: "Get version history for a document by path. Returns previous versions with timestamps, sources, and titles.",
	}, t.getDocumentVersions)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_memory",
		Description: "Create a new memory document. Memories are short notes stored under /memories/ with a date-prefixed path. Always adds the 'memory' label.",
	}, t.createMemory)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_document",
		Description: "Create a new document in the knowledge base. Content should be markdown. Fails if a document already exists at the path.",
	}, t.createDocument)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_document",
		Description: "Edit an existing document by replacing its full content. Read the document first with get_document, then pass the complete new content.",
	}, t.editDocument)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_document_section",
		Description: "Edit a specific section of a document by heading, without sending the full content. Use get_document with sections=true to see available sections. Supports replace, insert_after, insert_before, delete, and append operations.",
	}, t.editDocumentSection)
}

// ---------- Read tool handlers (iterate all vaults) ----------

type searchInput struct {
	Query   string   `json:"query" jsonschema:"Search query text"`
	Labels  []string `json:"labels,omitempty" jsonschema:"Filter by labels"`
	DocType *string  `json:"doc_type,omitempty" jsonschema:"Filter by document type"`
	Folder  *string  `json:"folder,omitempty" jsonschema:"Filter by folder path prefix"`
	Limit   *int     `json:"limit,omitempty" jsonschema:"Max results (default 20)"`
}

func (t *mcpTools) searchDocuments(ctx context.Context, req *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, nil, fmt.Errorf("query is required")
	}
	if input.Limit != nil && *input.Limit < 1 {
		return nil, nil, fmt.Errorf("limit must be positive")
	}

	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, err
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	var sb strings.Builder
	for _, vaultID := range vaultIDs {
		result, _, execErr := t.executor.ExecuteTool(ctx, vaultID, "search", string(argsJSON))
		if execErr != nil {
			slog.Warn("search failed", "vault", vaultID, "error", execErr)
			continue
		}
		if result != "" && result != "No results found." {
			sb.WriteString(result)
		}
	}

	if sb.Len() == 0 {
		return textResult("No results found."), nil, nil
	}
	return textResult(sb.String()), nil, nil
}

type getDocumentInput struct {
	Path     string `json:"path" jsonschema:"Document path"`
	Sections bool   `json:"sections,omitempty" jsonschema:"Include section outline for targeted editing with edit_document_section"`
}

func (t *mcpTools) getDocument(ctx context.Context, req *mcp.CallToolRequest, input getDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return nil, nil, fmt.Errorf("path is required")
	}

	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, err
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	for _, vaultID := range vaultIDs {
		result, _, execErr := t.executor.ExecuteTool(ctx, vaultID, "read_document", string(argsJSON))
		if execErr != nil {
			slog.Warn("get document failed", "vault", vaultID, "path", input.Path, "error", execErr)
			continue
		}
		if !strings.HasPrefix(result, "Document not found:") {
			return textResult(result), nil, nil
		}
	}

	return textResult(fmt.Sprintf("Document not found: %s", input.Path)), nil, nil
}

type listLabelsInput struct{}

func (t *mcpTools) listLabels(ctx context.Context, req *mcp.CallToolRequest, input listLabelsInput) (*mcp.CallToolResult, any, error) {
	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, err
	}

	labelSet := map[string]bool{}
	for _, vaultID := range vaultIDs {
		result, _, execErr := t.executor.ExecuteTool(ctx, vaultID, "list_labels", "{}")
		if execErr != nil {
			slog.Warn("list labels failed", "vault", vaultID, "error", execErr)
			continue
		}
		if result != "No labels found." {
			for _, l := range strings.Split(result, ", ") {
				labelSet[l] = true
			}
		}
	}

	if len(labelSet) == 0 {
		return textResult("No labels found."), nil, nil
	}

	labels := make([]string, 0, len(labelSet))
	for l := range labelSet {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	return textResult(strings.Join(labels, ", ")), nil, nil
}

type listFoldersInput struct{}

func (t *mcpTools) listFolders(ctx context.Context, req *mcp.CallToolRequest, input listFoldersInput) (*mcp.CallToolResult, any, error) {
	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, err
	}

	var sb strings.Builder
	for _, vaultID := range vaultIDs {
		result, _, execErr := t.executor.ExecuteTool(ctx, vaultID, "list_folders", "{}")
		if execErr != nil {
			slog.Warn("list folders failed", "vault", vaultID, "error", execErr)
			continue
		}
		if result != "No folders found." {
			sb.WriteString(result)
		}
	}

	if sb.Len() == 0 {
		return textResult("No folders found."), nil, nil
	}
	return textResult(sb.String()), nil, nil
}

type listFolderContentsInput struct {
	Folder string `json:"folder" jsonschema:"Folder path (e.g. /guides/)"`
}

func (t *mcpTools) listFolderContents(ctx context.Context, req *mcp.CallToolRequest, input listFolderContentsInput) (*mcp.CallToolResult, any, error) {
	if input.Folder == "" {
		return nil, nil, fmt.Errorf("folder is required")
	}

	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, err
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	var sb strings.Builder
	for _, vaultID := range vaultIDs {
		result, _, execErr := t.executor.ExecuteTool(ctx, vaultID, "list_folder_contents", string(argsJSON))
		if execErr != nil {
			slog.Warn("list folder contents failed", "vault", vaultID, "error", execErr)
			continue
		}
		if !strings.HasPrefix(result, "No contents found") {
			sb.WriteString(result)
		}
	}

	if sb.Len() == 0 {
		return textResult(fmt.Sprintf("No contents found in folder %s", input.Folder)), nil, nil
	}
	return textResult(sb.String()), nil, nil
}

type getDocumentVersionsInput struct {
	Path  string `json:"path" jsonschema:"Document path"`
	Limit *int   `json:"limit,omitempty" jsonschema:"Max versions to return (default 20)"`
}

func (t *mcpTools) getDocumentVersions(ctx context.Context, req *mcp.CallToolRequest, input getDocumentVersionsInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return nil, nil, fmt.Errorf("path is required")
	}
	if input.Limit != nil && *input.Limit < 1 {
		return nil, nil, fmt.Errorf("limit must be positive")
	}

	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, err
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	for _, vaultID := range vaultIDs {
		result, _, execErr := t.executor.ExecuteTool(ctx, vaultID, "get_document_versions", string(argsJSON))
		if execErr != nil {
			slog.Warn("get document versions failed", "vault", vaultID, "path", input.Path, "error", execErr)
			continue
		}
		if !strings.HasPrefix(result, "Document not found:") {
			return textResult(result), nil, nil
		}
	}

	return textResult(fmt.Sprintf("Document not found: %s", input.Path)), nil, nil
}

// ---------- Write tool handlers (use first vault) ----------

// executeWriteTool resolves vault access, marshals input, and executes a
// write tool on the first accessible vault.
func (t *mcpTools) executeWriteTool(ctx context.Context, toolName string, input any) (*mcp.CallToolResult, any, error) {
	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, err
	}
	if len(vaultIDs) == 0 {
		return nil, nil, fmt.Errorf("no vaults accessible")
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	result, _, execErr := t.executor.ExecuteTool(ctx, vaultIDs[0], toolName, string(argsJSON))
	if execErr != nil {
		return nil, nil, execErr
	}
	return textResult(result), nil, nil
}

type createMemoryInput struct {
	Title   string   `json:"title" jsonschema:"Memory title"`
	Content string   `json:"content" jsonschema:"Memory content (markdown)"`
	Labels  []string `json:"labels,omitempty" jsonschema:"Additional labels (memory label is always added)"`
}

func (t *mcpTools) createMemory(ctx context.Context, req *mcp.CallToolRequest, input createMemoryInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Title) == "" {
		return nil, nil, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}
	return t.executeWriteTool(ctx, "create_memory", input)
}

type createDocumentInput struct {
	Path    string `json:"path" jsonschema:"Document path (e.g. /guides/new-guide.md)"`
	Content string `json:"content" jsonschema:"Full markdown content"`
}

func (t *mcpTools) createDocument(ctx context.Context, req *mcp.CallToolRequest, input createDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return nil, nil, fmt.Errorf("path is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}
	return t.executeWriteTool(ctx, "create_document", input)
}

type editDocumentInput struct {
	Path    string `json:"path" jsonschema:"Document path of the existing document"`
	Content string `json:"content" jsonschema:"Complete new markdown content (replaces existing)"`
}

func (t *mcpTools) editDocument(ctx context.Context, req *mcp.CallToolRequest, input editDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return nil, nil, fmt.Errorf("path is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}
	return t.executeWriteTool(ctx, "edit_document", input)
}

type editDocumentSectionInput struct {
	Path       string  `json:"path" jsonschema:"Document path"`
	Operation  string  `json:"operation" jsonschema:"One of: replace, insert_after, insert_before, delete, append"`
	Heading    *string `json:"heading,omitempty" jsonschema:"Target section heading (empty string for preamble, omit for append)"`
	Position   *int    `json:"position,omitempty" jsonschema:"Disambiguation index for duplicate headings (default 0)"`
	Content    *string `json:"content,omitempty" jsonschema:"New section body (required for replace, insert, append)"`
	NewHeading *string `json:"new_heading,omitempty" jsonschema:"Heading text for insert/append operations"`
	NewLevel   *int    `json:"new_level,omitempty" jsonschema:"Heading level 1-6 for insert/append operations"`
}

func (t *mcpTools) editDocumentSection(ctx context.Context, req *mcp.CallToolRequest, input editDocumentSectionInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return nil, nil, fmt.Errorf("path is required")
	}
	if strings.TrimSpace(input.Operation) == "" {
		return nil, nil, fmt.Errorf("operation is required")
	}
	return t.executeWriteTool(ctx, "edit_document_section", input)
}

// ---------- helpers ----------

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}
