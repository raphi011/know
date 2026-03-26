package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/jina"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/memory"
	"github.com/raphi011/know/internal/metrics"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/render"
	"github.com/raphi011/know/internal/search"
)

// Canonical tool names. Use these constants instead of string literals.
const (
	ToolSearch              = "search"
	ToolReadDocument        = "read_document"
	ToolCreateDocument      = "create_document"
	ToolEditDocument        = "edit_document"
	ToolEditDocumentSection = "edit_document_section"
	ToolCreateMemory        = "create_memory"
	ToolListLabels          = "list_labels"
	ToolListFolders         = "list_folders"
	ToolListFolderContents  = "list_folder_contents"
	ToolGetDocumentVersions = "get_document_versions"
	ToolListTasks           = "list_tasks"
	ToolToggleTask          = "toggle_task"
	ToolFetchWebpage        = "fetch_webpage"
	ToolSearchDocuments     = "search_documents"
	ToolWebSearch           = "web_search"
)

// writeTools is the set of tools that require write approval.
var writeTools = map[string]bool{
	ToolCreateDocument:      true,
	ToolEditDocument:        true,
	ToolEditDocumentSection: true,
	ToolCreateMemory:        true,
}

// IsWriteTool reports whether the named tool requires write approval.
func IsWriteTool(name string) bool {
	return writeTools[name]
}

// ToolExecutor defines the interface for executing tools against a vault.
// Both the local Executor and the remote proxy implement this.
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, vaultID, toolName, arguments string) (string, *ToolResultMeta, error)
}

// Executor runs named tools against a single vault. It is the shared
// implementation used by both the agent chat and the embedded MCP server.
type Executor struct {
	DB        *db.Client
	Search    *search.Service
	FileSvc   *file.Service
	RenderSvc *render.Service
	Jina      *jina.Client
	Model     *llm.Model
	Metrics   *metrics.Metrics

	once     sync.Once
	registry map[string]tool.InvokableTool
}

// initRegistry lazily initializes the tool registry from the executor's
// services. Read-only tools are always registered; write tools require
// FileSvc to be non-nil.
func (e *Executor) initRegistry() {
	e.once.Do(func() {
		e.registry = map[string]tool.InvokableTool{
			ToolSearch:              &SearchTool{search: e.Search},
			ToolReadDocument:        &ReadDocumentTool{db: e.DB, fileSvc: e.FileSvc, renderSvc: e.RenderSvc},
			ToolListLabels:          &ListLabelsTool{db: e.DB},
			ToolListFolders:         &ListFoldersTool{db: e.DB},
			ToolListFolderContents:  &ListFolderContentsTool{db: e.DB},
			ToolGetDocumentVersions: &GetDocumentVersionsTool{db: e.DB},
			ToolListTasks:           &ListTasksTool{db: e.DB},
		}

		if e.FileSvc != nil {
			e.registry[ToolCreateDocument] = &CreateDocumentTool{db: e.DB, docService: e.FileSvc}
			e.registry[ToolEditDocument] = &EditDocumentTool{db: e.DB, docService: e.FileSvc}
			e.registry[ToolEditDocumentSection] = &EditDocumentSectionTool{db: e.DB, docService: e.FileSvc}
			e.registry[ToolCreateMemory] = &CreateMemoryTool{db: e.DB, docService: e.FileSvc}
			e.registry[ToolToggleTask] = &ToggleTaskTool{docService: e.FileSvc}
		}

		e.registry[ToolFetchWebpage] = &FetchWebpageTool{jina: e.Jina, db: e.DB, fileSvc: e.FileSvc, model: e.Model, metrics: e.Metrics}
	})
}

// Tools returns all registered tools as eino BaseTool implementations.
// The returned slice is suitable for use with compose.ToolsNodeConfig or
// model.WithTools.
func (e *Executor) Tools() []tool.BaseTool {
	e.initRegistry()
	out := make([]tool.BaseTool, 0, len(e.registry))
	for _, t := range e.registry {
		out = append(out, t)
	}
	return out
}

// ExecuteTool runs a tool by canonical name with JSON-encoded arguments
// scoped to a single vault. Returns the result text, optional metadata,
// and an error.
func (e *Executor) ExecuteTool(ctx context.Context, vaultID, toolName, arguments string) (_ string, _ *ToolResultMeta, err error) {
	e.initRegistry()

	t, ok := e.registry[toolName]
	if !ok {
		return "", nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
		}
		e.Metrics.RecordToolCall(toolName, status, time.Since(start))
	}()

	ctx = WithResultMeta(ctx)
	result, err := t.InvokableRun(ctx, arguments, WithVaultID(vaultID))
	if err != nil {
		return "", nil, fmt.Errorf("execute %s: %w", toolName, err)
	}

	return result, ResultMeta(ctx), nil
}

// BuildMemoryDocument delegates to memory.BuildMemoryDocument.
// Kept as an alias to avoid breaking remote.Executor imports.
func BuildMemoryDocument(project, title, content string, labels []string, settings models.VaultSettings) (path, fullContent string) {
	return memory.BuildMemoryDocument(project, title, content, labels, settings)
}

// checkContentHash validates optimistic concurrency. Returns a ToolError if
// hashes mismatch, or if the caller provided expected_hash but the document
// has no stored hash.
func checkContentHash(expectedHash, currentHash *string) error {
	if expectedHash == nil {
		return nil
	}
	if currentHash == nil {
		return &ToolError{Message: "document has no content hash; cannot verify expected_hash — re-read with get_document"}
	}
	if *expectedHash != *currentHash {
		return &ToolError{Message: fmt.Sprintf("document changed since you read it (expected hash %s, current %s), re-read with get_document", *expectedHash, *currentHash)}
	}
	return nil
}
