package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/raphi011/know/internal/logutil"
)

// MultiVaultReadTool fans out a read operation across all accessible vaults
// and merges the results. Remote results are prefixed with [namespace].
type MultiVaultReadTool struct {
	toolInfo *schema.ToolInfo
	resolver VaultResolver
	merge    mergeStrategy
}

// mergeStrategy controls how results from multiple vaults are combined.
type mergeStrategy int

const (
	// mergeConcat concatenates non-empty results from all vaults.
	mergeConcat mergeStrategy = iota
	// mergeFirstHit returns the first non-empty/non-"not found" result.
	mergeFirstHit
	// mergeDedupCSV deduplicates comma-separated values across vaults.
	mergeDedupCSV
)

func (t *MultiVaultReadTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return t.toolInfo, nil
}

func (t *MultiVaultReadTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	refs, err := t.resolver(ctx)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	switch t.merge {
	case mergeFirstHit:
		return t.runFirstHit(ctx, refs, argsJSON)
	case mergeDedupCSV:
		return t.runDedupCSV(ctx, refs, argsJSON)
	default:
		return t.runConcat(ctx, refs, argsJSON)
	}
}

func (t *MultiVaultReadTool) runConcat(ctx context.Context, refs []VaultRef, argsJSON string) (string, error) {
	logger := logutil.FromCtx(ctx)
	var sb strings.Builder
	var failedVaults int

	for _, ref := range refs {
		result, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, t.toolInfo.Name, argsJSON)
		if execErr != nil {
			logger.Warn("multi-vault tool failed", "tool", t.toolInfo.Name, "vault", ref.VaultID, "namespace", ref.Namespace, "error", execErr)
			failedVaults++
			continue
		}
		if result != "" && !isEmptyResult(result) {
			if ref.IsRemote() {
				fmt.Fprintf(&sb, "[%s]\n", ref.Namespace)
			}
			sb.WriteString(result)
			if !strings.HasSuffix(result, "\n") {
				sb.WriteByte('\n')
			}
		}
	}

	if sb.Len() == 0 {
		if failedVaults > 0 {
			return fmt.Sprintf("No results found. Note: %d vault(s) were unreachable and could not be queried.", failedVaults), nil
		}
		return "No results found.", nil
	}
	return sb.String(), nil
}

func (t *MultiVaultReadTool) runFirstHit(ctx context.Context, refs []VaultRef, argsJSON string) (string, error) {
	logger := logutil.FromCtx(ctx)
	var failedVaults int

	for _, ref := range refs {
		result, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, t.toolInfo.Name, argsJSON)
		if execErr != nil {
			var toolErr *ToolError
			if errors.As(execErr, &toolErr) {
				continue // not found in this vault, try next
			}
			logger.Warn("multi-vault tool failed", "tool", t.toolInfo.Name, "vault", ref.VaultID, "namespace", ref.Namespace, "error", execErr)
			failedVaults++
			continue
		}
		if result != "" && !isNotFoundResult(result) {
			if ref.IsRemote() {
				result = fmt.Sprintf("[%s]\n%s", ref.Namespace, result)
			}
			return result, nil
		}
	}

	if failedVaults > 0 {
		return fmt.Sprintf("No results found. Note: %d vault(s) were unreachable and could not be queried.", failedVaults), nil
	}
	return "No results found.", nil
}

func (t *MultiVaultReadTool) runDedupCSV(ctx context.Context, refs []VaultRef, argsJSON string) (string, error) {
	logger := logutil.FromCtx(ctx)
	seen := map[string]bool{}
	var failedVaults int

	for _, ref := range refs {
		result, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, t.toolInfo.Name, argsJSON)
		if execErr != nil {
			logger.Warn("multi-vault tool failed", "tool", t.toolInfo.Name, "vault", ref.VaultID, "namespace", ref.Namespace, "error", execErr)
			failedVaults++
			continue
		}
		if result != "" && !isEmptyResult(result) {
			for item := range strings.SplitSeq(result, ", ") {
				seen[item] = true
			}
		}
	}

	if len(seen) == 0 {
		if failedVaults > 0 {
			return fmt.Sprintf("No results found. Note: %d vault(s) were unreachable and could not be queried.", failedVaults), nil
		}
		return "No results found.", nil
	}

	items := make([]string, 0, len(seen))
	for item := range seen {
		items = append(items, item)
	}
	return strings.Join(items, ", "), nil
}

// MultiVaultWriteTool routes a write operation to a single vault resolved by name.
type MultiVaultWriteTool struct {
	toolInfo      *schema.ToolInfo
	writeResolver WriteVaultResolver
}

func (t *MultiVaultWriteTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return t.toolInfo, nil
}

func (t *MultiVaultWriteTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	// Extract optional "vault" field from args to determine target vault.
	var args struct {
		Vault string `json:"vault"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("error: parse args: %v", err), nil
	}

	ref, err := t.writeResolver(ctx, args.Vault)
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}

	result, _, execErr := ref.Executor.ExecuteTool(ctx, ref.VaultID, t.toolInfo.Name, argsJSON)
	if execErr != nil {
		var toolErr *ToolError
		if errors.As(execErr, &toolErr) {
			return fmt.Sprintf("error: %s", toolErr.Message), nil
		}
		return fmt.Sprintf("error: %v", execErr), nil
	}
	if ref.IsRemote() {
		result = fmt.Sprintf("[%s] %s", ref.Namespace, result)
	}
	return result, nil
}

// NewMultiVaultTools returns all knowledge-base tools as multi-vault wrappers.
// Read tools fan out across all vaults; write tools route to a specified vault.
func NewMultiVaultTools(resolver VaultResolver, writeResolver WriteVaultResolver) []tool.BaseTool {
	readTools := []struct {
		info  *schema.ToolInfo
		merge mergeStrategy
	}{
		{
			info: &schema.ToolInfo{
				Name: "search",
				Desc: "Search documents using full-text and semantic search. Returns titles, paths, scores, and matching snippets. Use list_labels first to discover labels for filtering.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"query":    {Type: schema.String, Desc: "Search query text", Required: true},
					"labels":   {Type: schema.Array, Desc: "Filter by labels (call list_labels to discover)", ElemInfo: &schema.ParameterInfo{Type: schema.String}},
					"doc_type": {Type: schema.String, Desc: "Filter by document type (e.g. note, guide)"},
					"folder":   {Type: schema.String, Desc: "Filter by folder path prefix (e.g. /guides/)"},
					"limit":    {Type: schema.Integer, Desc: "Max results (default 20, max 100)"},
				}),
			},
			merge: mergeConcat,
		},
		{
			info: &schema.ToolInfo{
				Name: "read_document",
				Desc: "Read the full content of a specific document by its path. Set sections=true to include a section outline for use with edit_document_section.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"path":     {Type: schema.String, Desc: "The document path (e.g. /folder/document-name)", Required: true},
					"sections": {Type: schema.Boolean, Desc: "Include section outline for targeted editing"},
				}),
			},
			merge: mergeFirstHit,
		},
		{
			info: &schema.ToolInfo{
				Name:        "list_labels",
				Desc:        "List all labels/categories used across documents in all vaults",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{}),
			},
			merge: mergeDedupCSV,
		},
		{
			info: &schema.ToolInfo{
				Name: "list_folders",
				Desc: "List the folder structure across all vaults",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"parent": {Type: schema.String, Desc: "Parent folder path to list children of (e.g. /guides/). Lists all folders if omitted."},
				}),
			},
			merge: mergeConcat,
		},
		{
			info: &schema.ToolInfo{
				Name: "list_folder_contents",
				Desc: "List documents and subfolders in a specific folder. Returns immediate children only.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"folder": {Type: schema.String, Desc: "Folder path (e.g. /guides/)", Required: true},
				}),
			},
			merge: mergeConcat,
		},
		{
			info: &schema.ToolInfo{
				Name: "get_document_versions",
				Desc: "Get version history for a document by path. Returns previous versions with timestamps, sources, and titles.",
				ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
					"path":  {Type: schema.String, Desc: "Document path", Required: true},
					"limit": {Type: schema.Integer, Desc: "Max versions to return (default 20, max 100)"},
				}),
			},
			merge: mergeFirstHit,
		},
	}

	writeTools := []*schema.ToolInfo{
		{
			Name: "create_document",
			Desc: "Create a new document in the knowledge base. The content should be markdown. Fails if a document already exists at the given path.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path":    {Type: schema.String, Desc: "Document path (e.g. /guides/new-guide.md)", Required: true},
				"content": {Type: schema.String, Desc: "Full markdown content for the document", Required: true},
				"vault":   {Type: schema.String, Desc: "Target vault (e.g. remote-name/vault-name). Defaults to first local vault."},
			}),
		},
		{
			Name: "edit_document",
			Desc: "Edit an existing document by replacing its full content. Read the document first to get the current content, then modify and pass the complete new content.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path":          {Type: schema.String, Desc: "Document path of the existing document", Required: true},
				"content":       {Type: schema.String, Desc: "Complete new markdown content (replaces existing content entirely)", Required: true},
				"expected_hash": {Type: schema.String, Desc: "Content hash from get_document for optimistic concurrency check"},
				"vault":         {Type: schema.String, Desc: "Target vault (e.g. remote-name/vault-name). Defaults to first local vault."},
			}),
		},
		{
			Name: "edit_document_section",
			Desc: "Edit a specific section of a document by heading, without sending the full content. Use get_document with sections=true to see available sections. Supports replace, insert_after, insert_before, delete, and append operations.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path":          {Type: schema.String, Desc: "Document path", Required: true},
				"operation":     {Type: schema.String, Desc: "One of: replace, insert_after, insert_before, delete, append", Required: true},
				"heading":       {Type: schema.String, Desc: "Target section heading (empty string for preamble, omit for append)"},
				"position":      {Type: schema.Integer, Desc: "Disambiguation index for duplicate headings (default 0)"},
				"content":       {Type: schema.String, Desc: "New section body (required for replace, insert, append)"},
				"new_heading":   {Type: schema.String, Desc: "Heading text for insert/append operations"},
				"new_level":     {Type: schema.Integer, Desc: "Heading level 1-6 for insert/append operations"},
				"expected_hash": {Type: schema.String, Desc: "Content hash from get_document for optimistic concurrency check"},
				"vault":         {Type: schema.String, Desc: "Target vault (e.g. remote-name/vault-name). Defaults to first local vault."},
			}),
		},
		{
			Name: "create_memory",
			Desc: "Create a memory, optionally scoped to a project. For project memories, use a stable identifier (git remote URL or repo folder name). For global memories, omit project and add descriptive labels. Call list_labels first to reuse existing labels.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"title":   {Type: schema.String, Desc: "Memory title (used for filename)", Required: true},
				"content": {Type: schema.String, Desc: "Memory content (markdown)", Required: true},
				"project": {Type: schema.String, Desc: "Project identifier (git remote URL or repo folder name). Omit for global memories."},
				"labels":  {Type: schema.Array, Desc: "Labels for categorization (e.g. golang, docker). Call list_labels to discover existing labels.", ElemInfo: &schema.ParameterInfo{Type: schema.String}},
				"vault":   {Type: schema.String, Desc: "Target vault (e.g. remote-name/vault-name). Defaults to first local vault."},
			}),
		},
	}

	out := make([]tool.BaseTool, 0, len(readTools)+len(writeTools))
	for _, rt := range readTools {
		out = append(out, &MultiVaultReadTool{
			toolInfo: rt.info,
			resolver: resolver,
			merge:    rt.merge,
		})
	}
	for _, info := range writeTools {
		out = append(out, &MultiVaultWriteTool{
			toolInfo:      info,
			writeResolver: writeResolver,
		})
	}
	return out
}

// isEmptyResult checks for common "no results" responses from underlying tools.
func isEmptyResult(result string) bool {
	return result == "No results found." ||
		result == "No labels found." ||
		result == "No folders found."
}

// isNotFoundResult checks for "not found" responses that indicate the item
// doesn't exist in this vault (but might exist in another).
func isNotFoundResult(result string) bool {
	return isEmptyResult(result)
}
