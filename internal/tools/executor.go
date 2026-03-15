package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/document"
	"github.com/raphi011/know/internal/memory"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/search"
)

// ToolExecutor defines the interface for executing tools against a vault.
// Both the local Executor and the remote proxy implement this.
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, vaultID, toolName, arguments string) (string, *ToolResultMeta, error)
}

// Executor runs named tools against a single vault. It is the shared
// implementation used by both the agent chat and the embedded MCP server.
type Executor struct {
	DB         *db.Client
	Search     *search.Service
	DocService *document.Service

	once     sync.Once
	registry map[string]tool.InvokableTool
}

// initRegistry lazily initializes the tool registry from the executor's
// services. Read-only tools are always registered; write tools require
// DocService to be non-nil.
func (e *Executor) initRegistry() {
	e.once.Do(func() {
		e.registry = map[string]tool.InvokableTool{
			"search":                &SearchTool{search: e.Search},
			"read_document":         &ReadDocumentTool{db: e.DB},
			"list_labels":           &ListLabelsTool{db: e.DB},
			"list_folders":          &ListFoldersTool{db: e.DB},
			"list_folder_contents":  &ListFolderContentsTool{db: e.DB},
			"get_document_versions": &GetDocumentVersionsTool{db: e.DB},
			"list_tasks":            &ListTasksTool{db: e.DB},
		}

		if e.DocService != nil {
			e.registry["create_document"] = &CreateDocumentTool{db: e.DB, docService: e.DocService}
			e.registry["edit_document"] = &EditDocumentTool{db: e.DB, docService: e.DocService}
			e.registry["edit_document_section"] = &EditDocumentSectionTool{db: e.DB, docService: e.DocService}
			e.registry["create_memory"] = &CreateMemoryTool{db: e.DB, docService: e.DocService}
			e.registry["toggle_task"] = &ToggleTaskTool{docService: e.DocService}
		}
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
func (e *Executor) ExecuteTool(ctx context.Context, vaultID, toolName, arguments string) (string, *ToolResultMeta, error) {
	e.initRegistry()

	t, ok := e.registry[toolName]
	if !ok {
		return "", nil, fmt.Errorf("unknown tool: %s", toolName)
	}

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
