package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/memory"
	"github.com/raphi011/know/internal/remote"
	"github.com/raphi011/know/internal/tools"
	"github.com/raphi011/know/internal/vault"
)

type mcpTools struct {
	executor      tools.ToolExecutor
	db            *db.Client
	vaultService  *vault.Service
	remoteService *remote.Service
	memoryService *memory.Service
	cache         *cache
}

func (t *mcpTools) register(server *mcp.Server) {
	readOnly := &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: new(false),
		OpenWorldHint:   new(false),
	}
	writeNonDestructive := &mcp.ToolAnnotations{
		DestructiveHint: new(false),
		OpenWorldHint:   new(false),
	}
	writeIdempotent := &mcp.ToolAnnotations{
		DestructiveHint: new(false),
		IdempotentHint:  true,
		OpenWorldHint:   new(false),
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

	writeDestructive := &mcp.ToolAnnotations{
		DestructiveHint: new(true),
		OpenWorldHint:   new(false),
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_memory",
		Description: "Create a memory, optionally scoped to a project. Automatically labels with 'memory' (and 'project/{project}' if project is set). For project memories, use a stable identifier (git remote URL or repo folder name). For global memories (e.g. Go patterns, Docker tips), omit project and add descriptive labels for categorization. Always call list_labels first to discover existing labels and reuse them for consistency.",
		Annotations: writeNonDestructive,
	}, t.createMemory)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "retrieve_memories",
		Description: "Retrieve memories sorted by relevance. Supports three modes: project-scoped (set project), label-filtered (set labels, e.g. 'golang'), or both. Uses decay scoring (recency + access frequency) to rank memories. Stale memories are auto-archived. Similar memories may be consolidated. Call at session start to load project context and relevant global memories.",
		Annotations: writeNonDestructive,
	}, t.retrieveMemories)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_memory",
		Description: "Delete a memory by path. Use when a memory is known to be outdated or incorrect.",
		Annotations: writeDestructive,
	}, t.deleteMemory)

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

	refs, err := t.resolveAllVaults(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("search documents: %w", err)
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	var sb strings.Builder
	for _, ref := range refs {
		result, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, "search", string(argsJSON))
		if execErr != nil {
			logutil.FromCtx(ctx).Warn("search failed", "vault", ref.VaultID, "namespace", ref.Namespace, "error", execErr)
			continue
		}
		if result != "" && result != "No results found." {
			if ref.IsRemote() {
				fmt.Fprintf(&sb, "[%s]\n", ref.Namespace)
			}
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

	refs, err := t.resolveAllVaults(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get document: %w", err)
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	for _, ref := range refs {
		result, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, "read_document", string(argsJSON))
		if execErr != nil {
			logutil.FromCtx(ctx).Warn("get document failed", "vault", ref.VaultID, "namespace", ref.Namespace, "path", input.Path, "error", execErr)
			continue
		}
		if !strings.HasPrefix(result, "Document not found:") {
			if ref.IsRemote() {
				result = fmt.Sprintf("[%s]\n%s", ref.Namespace, result)
			}
			return textResult(result), nil, nil
		}
	}

	return errorResult(fmt.Sprintf("Document not found: %s. Use search_documents to find it, or list_folder_contents to browse.", input.Path)), nil, nil
}

type listLabelsInput struct{}

func (t *mcpTools) listLabels(ctx context.Context, req *mcp.CallToolRequest, input listLabelsInput) (*mcp.CallToolResult, any, error) {
	refs, err := t.resolveAllVaults(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list labels: %w", err)
	}

	labelSet := map[string]bool{}
	for _, ref := range refs {
		cacheKey := "list_labels:" + ref.Namespace + ":" + ref.VaultID
		result, err := t.cache.GetOrFetch(cacheKey, func() (string, error) {
			r, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, "list_labels", "{}")
			return r, execErr
		})
		if err != nil {
			logutil.FromCtx(ctx).Warn("list labels failed", "vault", ref.VaultID, "namespace", ref.Namespace, "error", err)
			continue
		}
		if result != "No labels found." {
			for l := range strings.SplitSeq(result, ", ") {
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
	refs, err := t.resolveAllVaults(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list folders: %w", err)
	}

	var sb strings.Builder
	for _, ref := range refs {
		cacheKey := "list_folders:" + ref.Namespace + ":" + ref.VaultID
		result, err := t.cache.GetOrFetch(cacheKey, func() (string, error) {
			r, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, "list_folders", "{}")
			return r, execErr
		})
		if err != nil {
			logutil.FromCtx(ctx).Warn("list folders failed", "vault", ref.VaultID, "namespace", ref.Namespace, "error", err)
			continue
		}
		if result != "No folders found." {
			if ref.IsRemote() {
				fmt.Fprintf(&sb, "[%s]\n", ref.Namespace)
			}
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

	refs, err := t.resolveAllVaults(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list folder contents: %w", err)
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	var sb strings.Builder
	for _, ref := range refs {
		result, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, "list_folder_contents", string(argsJSON))
		if execErr != nil {
			logutil.FromCtx(ctx).Warn("list folder contents failed", "vault", ref.VaultID, "namespace", ref.Namespace, "error", execErr)
			continue
		}
		if !strings.HasPrefix(result, "No contents found") {
			if ref.IsRemote() {
				fmt.Fprintf(&sb, "[%s]\n", ref.Namespace)
			}
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

	refs, err := t.resolveAllVaults(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get document versions: %w", err)
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	for _, ref := range refs {
		result, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, "get_document_versions", string(argsJSON))
		if execErr != nil {
			logutil.FromCtx(ctx).Warn("get document versions failed", "vault", ref.VaultID, "namespace", ref.Namespace, "path", input.Path, "error", execErr)
			continue
		}
		if !strings.HasPrefix(result, "Document not found:") {
			if ref.IsRemote() {
				result = fmt.Sprintf("[%s]\n%s", ref.Namespace, result)
			}
			return textResult(result), nil, nil
		}
	}

	return errorResult(fmt.Sprintf("Document not found: %s. Use search_documents to find it, or list_folder_contents to browse.", input.Path)), nil, nil
}

// ---------- Write tool handlers (use first vault) ----------

// executeWriteTool resolves vault access, marshals input, and executes a
// write tool on the target vault. If vaultName contains "/" it routes to
// a remote vault; otherwise it uses the first local vault.
func (t *mcpTools) executeWriteTool(ctx context.Context, toolName, vaultName string, input any) (*mcp.CallToolResult, any, error) {
	ref, err := t.resolveWriteVault(ctx, vaultName)
	if err != nil {
		return nil, nil, fmt.Errorf("execute write tool %s: %w", toolName, err)
	}

	argsJSON, err := json.Marshal(input)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal args: %w", err)
	}

	result, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, toolName, string(argsJSON))
	if execErr != nil {
		if isToolLevelError(execErr) {
			return errorResult(execErr.Error()), nil, nil
		}
		return nil, nil, fmt.Errorf("execute write tool %s: %w", toolName, execErr)
	}
	if ref.IsRemote() {
		result = fmt.Sprintf("[%s] %s", ref.Namespace, result)
	}
	return textResult(result), nil, nil
}

type createMemoryInput struct {
	Project string   `json:"project,omitempty" jsonschema:"Optional project identifier. Use the git remote origin URL or repository folder name for project-scoped memories. Omit for global memories (e.g. general Go patterns)."`
	Title   string   `json:"title" jsonschema:"Memory title (used for filename)"`
	Content string   `json:"content" jsonschema:"Memory content (markdown)"`
	Labels  []string `json:"labels,omitempty" jsonschema:"Additional labels for categorization (e.g. golang, docker, debugging). Call list_labels first to reuse existing labels. Especially important for global (non-project) memories."`
	Vault   string   `json:"vault,omitempty" jsonschema:"Target vault (e.g. home/default for remote). Defaults to first local vault."`
}

func (t *mcpTools) createMemory(ctx context.Context, req *mcp.CallToolRequest, input createMemoryInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Title) == "" {
		return errorResult("title is required"), nil, nil
	}
	if strings.TrimSpace(input.Content) == "" {
		return errorResult("content is required"), nil, nil
	}
	return t.executeWriteTool(ctx, "create_memory", input.Vault, input)
}

type retrieveMemoriesInput struct {
	Project         string   `json:"project,omitempty" jsonschema:"Project identifier (same value used when creating memories). Omit to retrieve all memories or filter by labels only."`
	Labels          []string `json:"labels,omitempty" jsonschema:"Filter by additional labels (e.g. golang, docker). Combined with project filter using AND logic."`
	IncludeArchived bool     `json:"include_archived,omitempty" jsonschema:"Include archived (low-scoring) memories. Default false."`
	Vault           string   `json:"vault,omitempty" jsonschema:"Target vault. Defaults to first local vault."`
}

func (t *mcpTools) retrieveMemories(ctx context.Context, req *mcp.CallToolRequest, input retrieveMemoriesInput) (*mcp.CallToolResult, any, error) {
	if t.memoryService == nil {
		return errorResult("memory service not configured"), nil, nil
	}

	ref, err := t.resolveWriteVault(ctx, input.Vault)
	if err != nil {
		return nil, nil, fmt.Errorf("retrieve memories: %w", err)
	}

	// Load vault for settings
	v, err := t.db.GetVault(ctx, ref.VaultID)
	if err != nil {
		return nil, nil, fmt.Errorf("retrieve memories: load vault: %w", err)
	}
	if v == nil {
		return errorResult("vault not found"), nil, nil
	}
	settings := v.MemoryDefaults()

	memories, err := t.memoryService.Retrieve(ctx, ref.VaultID, input.Project, input.Labels, input.IncludeArchived, settings)
	if err != nil {
		return nil, nil, fmt.Errorf("retrieve memories: %w", err)
	}

	if len(memories) == 0 {
		scope := "project: " + input.Project
		if input.Project == "" {
			scope = "all memories"
			if len(input.Labels) > 0 {
				scope = "labels: " + strings.Join(input.Labels, ", ")
			}
		}
		return textResult("No memories found for " + scope), nil, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d memories:\n\n", len(memories))
	for _, m := range memories {
		fmt.Fprintf(&sb, "--- %s (score: %.2f) ---\n", m.Document.Path, m.Score)
		fmt.Fprintf(&sb, "%s\n\n", m.Document.ContentBody)
	}
	return textResult(sb.String()), nil, nil
}

type deleteMemoryInput struct {
	Path  string `json:"path" jsonschema:"Full path of the memory to delete"`
	Vault string `json:"vault,omitempty" jsonschema:"Target vault. Defaults to first local vault."`
}

func (t *mcpTools) deleteMemory(ctx context.Context, req *mcp.CallToolRequest, input deleteMemoryInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return errorResult("path is required"), nil, nil
	}

	if t.memoryService == nil {
		return errorResult("memory service not configured"), nil, nil
	}

	ref, err := t.resolveWriteVault(ctx, input.Vault)
	if err != nil {
		return nil, nil, fmt.Errorf("delete memory: %w", err)
	}

	if err := t.memoryService.Delete(ctx, ref.VaultID, input.Path); err != nil {
		return nil, nil, fmt.Errorf("delete memory: %w", err)
	}

	return textResult(fmt.Sprintf("Memory deleted: %s", input.Path)), nil, nil
}

type createDocumentInput struct {
	Path    string `json:"path" jsonschema:"Document path (e.g. /guides/new-guide.md)"`
	Content string `json:"content" jsonschema:"Full markdown content"`
	Vault   string `json:"vault,omitempty" jsonschema:"Target vault (e.g. home/default for remote). Defaults to first local vault."`
}

func (t *mcpTools) createDocument(ctx context.Context, req *mcp.CallToolRequest, input createDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return errorResult("path is required"), nil, nil
	}
	if strings.TrimSpace(input.Content) == "" {
		return errorResult("content is required"), nil, nil
	}
	return t.executeWriteTool(ctx, "create_document", input.Vault, input)
}

type editDocumentInput struct {
	Path         string  `json:"path" jsonschema:"Document path of the existing document"`
	Content      string  `json:"content" jsonschema:"Complete new markdown content (replaces existing)"`
	ExpectedHash *string `json:"expected_hash,omitempty" jsonschema:"Content hash from get_document for optimistic concurrency check"`
	Vault        string  `json:"vault,omitempty" jsonschema:"Target vault (e.g. home/default for remote). Defaults to first local vault."`
}

func (t *mcpTools) editDocument(ctx context.Context, req *mcp.CallToolRequest, input editDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return errorResult("path is required"), nil, nil
	}
	if strings.TrimSpace(input.Content) == "" {
		return errorResult("content is required"), nil, nil
	}
	return t.executeWriteTool(ctx, "edit_document", input.Vault, input)
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
	Vault        string  `json:"vault,omitempty" jsonschema:"Target vault (e.g. home/default for remote). Defaults to first local vault."`
}

func (t *mcpTools) editDocumentSection(ctx context.Context, req *mcp.CallToolRequest, input editDocumentSectionInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return errorResult("path is required"), nil, nil
	}
	if strings.TrimSpace(input.Operation) == "" {
		return errorResult("operation is required. Use one of: replace, insert_after, insert_before, delete, append"), nil, nil
	}
	return t.executeWriteTool(ctx, "edit_document_section", input.Vault, input)
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

// isToolLevelError returns true for executor errors that are user-correctable
// and should be returned as MCP tool errors (IsError=true) rather than
// infrastructure errors.
func isToolLevelError(err error) bool {
	var toolErr *tools.ToolError
	return errors.As(err, &toolErr)
}
