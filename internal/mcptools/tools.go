package mcptools

import (
	"context"
	"encoding/json"
	"errors"
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
	cache        *cache
}

func (t *mcpTools) register(server *mcp.Server) {
	readOnly := &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
	}
	writeNonDestructive := &mcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
	}
	writeIdempotent := &mcp.ToolAnnotations{
		DestructiveHint: boolPtr(false),
		IdempotentHint:  true,
		OpenWorldHint:   boolPtr(false),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_documents",
		Description: "Search documents using full-text and semantic search. Returns titles, paths, scores, and matching snippets.",
		Annotations: readOnly,
	}, t.searchDocuments)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_document",
		Description: "Get a document by its path. Returns the full content, title, labels, content hash, and metadata. The content hash can be passed to edit_document or edit_document_section as expected_hash for optimistic concurrency. Set sections=true to include a section outline for use with edit_document_section.",
		Annotations: readOnly,
	}, t.getDocument)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_labels",
		Description: "List all labels used across documents. Useful for discovering available categories before searching.",
		Annotations: readOnly,
	}, t.listLabels)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_folders",
		Description: "List the folder structure. Use to browse and understand vault organization before searching.",
		Annotations: readOnly,
	}, t.listFolders)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_folder_contents",
		Description: "List documents and subfolders in a specific folder. Returns immediate children only.",
		Annotations: readOnly,
	}, t.listFolderContents)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_document_versions",
		Description: "Get version history for a document by path. Returns previous versions with timestamps, sources, and titles.",
		Annotations: readOnly,
	}, t.getDocumentVersions)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_memory",
		Description: "Create a new memory document. Memories are short notes stored under /memories/ with a date-prefixed path. Always adds the 'memory' label.",
		Annotations: writeNonDestructive,
	}, t.createMemory)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_document",
		Description: "Create a new document in the knowledge base. Content should be markdown. Fails if a document already exists at the path.",
		Annotations: writeNonDestructive,
	}, t.createDocument)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_document",
		Description: "Edit an existing document by replacing its full content. Read the document first with get_document, then pass the complete new content. Optionally pass expected_hash (from get_document) to prevent overwriting concurrent changes.",
		Annotations: writeIdempotent,
	}, t.editDocument)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "edit_document_section",
		Description: "Edit a specific section of a document by heading, without sending the full content. Use get_document with sections=true to see available sections. Supports replace, insert_after, insert_before, delete, and append operations. Optionally pass expected_hash to prevent overwriting concurrent changes.",
		Annotations: writeIdempotent,
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
		return errorResult("query is required"), nil, nil
	}
	if input.Limit != nil && *input.Limit < 1 {
		return errorResult("limit must be positive"), nil, nil
	}

	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, fmt.Errorf("search documents: %w", err)
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
		return errorResult("path is required"), nil, nil
	}

	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, fmt.Errorf("get document: %w", err)
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

	return errorResult(fmt.Sprintf("Document not found: %s. Use search_documents to find it, or list_folder_contents to browse.", input.Path)), nil, nil
}

type listLabelsInput struct{}

func (t *mcpTools) listLabels(ctx context.Context, req *mcp.CallToolRequest, input listLabelsInput) (*mcp.CallToolResult, any, error) {
	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, fmt.Errorf("list labels: %w", err)
	}

	labelSet := map[string]bool{}
	for _, vaultID := range vaultIDs {
		result, err := t.cache.GetOrFetch("list_labels:"+vaultID, func() (string, error) {
			r, _, execErr := t.executor.ExecuteTool(ctx, vaultID, "list_labels", "{}")
			return r, execErr
		})
		if err != nil {
			slog.Warn("list labels failed", "vault", vaultID, "error", err)
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
		return nil, nil, fmt.Errorf("list folders: %w", err)
	}

	var sb strings.Builder
	for _, vaultID := range vaultIDs {
		result, err := t.cache.GetOrFetch("list_folders:"+vaultID, func() (string, error) {
			r, _, execErr := t.executor.ExecuteTool(ctx, vaultID, "list_folders", "{}")
			return r, execErr
		})
		if err != nil {
			slog.Warn("list folders failed", "vault", vaultID, "error", err)
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
		return errorResult("folder is required"), nil, nil
	}

	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, fmt.Errorf("list folder contents: %w", err)
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
		return errorResult(fmt.Sprintf("No contents found in folder %s. Use list_folders to see available folders.", input.Folder)), nil, nil
	}
	return textResult(sb.String()), nil, nil
}

type getDocumentVersionsInput struct {
	Path  string `json:"path" jsonschema:"Document path"`
	Limit *int   `json:"limit,omitempty" jsonschema:"Max versions to return (default 20)"`
}

func (t *mcpTools) getDocumentVersions(ctx context.Context, req *mcp.CallToolRequest, input getDocumentVersionsInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return errorResult("path is required"), nil, nil
	}
	if input.Limit != nil && *input.Limit < 1 {
		return errorResult("limit must be positive"), nil, nil
	}

	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, fmt.Errorf("get document versions: %w", err)
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

	return errorResult(fmt.Sprintf("Document not found: %s. Use search_documents to find it, or list_folder_contents to browse.", input.Path)), nil, nil
}

// ---------- Write tool handlers (use first vault) ----------

// executeWriteTool resolves vault access, marshals input, and executes a
// write tool on the first accessible vault.
func (t *mcpTools) executeWriteTool(ctx context.Context, toolName string, input any) (*mcp.CallToolResult, any, error) {
	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, nil, fmt.Errorf("execute write tool %s: %w", toolName, err)
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
		if isToolLevelError(execErr) {
			return errorResult(execErr.Error()), nil, nil
		}
		return nil, nil, fmt.Errorf("execute write tool %s: %w", toolName, execErr)
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
		return errorResult("title is required"), nil, nil
	}
	if strings.TrimSpace(input.Content) == "" {
		return errorResult("content is required"), nil, nil
	}
	return t.executeWriteTool(ctx, "create_memory", input)
}

type createDocumentInput struct {
	Path    string `json:"path" jsonschema:"Document path (e.g. /guides/new-guide.md)"`
	Content string `json:"content" jsonschema:"Full markdown content"`
}

func (t *mcpTools) createDocument(ctx context.Context, req *mcp.CallToolRequest, input createDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return errorResult("path is required"), nil, nil
	}
	if strings.TrimSpace(input.Content) == "" {
		return errorResult("content is required"), nil, nil
	}
	return t.executeWriteTool(ctx, "create_document", input)
}

type editDocumentInput struct {
	Path         string  `json:"path" jsonschema:"Document path of the existing document"`
	Content      string  `json:"content" jsonschema:"Complete new markdown content (replaces existing)"`
	ExpectedHash *string `json:"expected_hash,omitempty" jsonschema:"Content hash from get_document for optimistic concurrency check"`
}

func (t *mcpTools) editDocument(ctx context.Context, req *mcp.CallToolRequest, input editDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return errorResult("path is required"), nil, nil
	}
	if strings.TrimSpace(input.Content) == "" {
		return errorResult("content is required"), nil, nil
	}
	return t.executeWriteTool(ctx, "edit_document", input)
}

type editDocumentSectionInput struct {
	Path         string  `json:"path" jsonschema:"Document path"`
	Operation    string  `json:"operation" jsonschema:"One of: replace, insert_after, insert_before, delete, append"`
	Heading      *string `json:"heading,omitempty" jsonschema:"Target section heading (empty string for preamble, omit for append)"`
	Position     *int    `json:"position,omitempty" jsonschema:"Disambiguation index for duplicate headings (default 0)"`
	Content      *string `json:"content,omitempty" jsonschema:"New section body (required for replace, insert, append)"`
	NewHeading   *string `json:"new_heading,omitempty" jsonschema:"Heading text for insert/append operations"`
	NewLevel     *int    `json:"new_level,omitempty" jsonschema:"Heading level 1-6 for insert/append operations"`
	ExpectedHash *string `json:"expected_hash,omitempty" jsonschema:"Content hash from get_document for optimistic concurrency check"`
}

func (t *mcpTools) editDocumentSection(ctx context.Context, req *mcp.CallToolRequest, input editDocumentSectionInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return errorResult("path is required"), nil, nil
	}
	if strings.TrimSpace(input.Operation) == "" {
		return errorResult("operation is required. Use one of: replace, insert_after, insert_before, delete, append"), nil, nil
	}
	return t.executeWriteTool(ctx, "edit_document_section", input)
}

// ---------- helpers ----------

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

func errorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}

func boolPtr(b bool) *bool { return &b }

// isToolLevelError returns true for executor errors that are user-correctable
// and should be returned as MCP tool errors (IsError=true) rather than
// infrastructure errors.
func isToolLevelError(err error) bool {
	var toolErr *tools.ToolError
	return errors.As(err, &toolErr)
}
