package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/tools"
)

// Executor implements tools.ToolExecutor by proxying tool calls to a remote
// know server via its REST API.
type Executor struct {
	client     *apiclient.Client
	remoteName string
}

// NewExecutor creates a remote executor for the given remote.
func NewExecutor(client *apiclient.Client, remoteName string) *Executor {
	return &Executor{client: client, remoteName: remoteName}
}

// ExecuteTool routes a tool call to the appropriate REST API method on the remote server.
func (e *Executor) ExecuteTool(ctx context.Context, vaultID, toolName, arguments string) (string, *tools.ToolResultMeta, error) {
	switch toolName {
	case "search":
		return e.execSearch(ctx, vaultID, arguments)
	case "read_document":
		return e.execReadDocument(ctx, vaultID, arguments)
	case "list_labels":
		return e.execListLabels(ctx, vaultID)
	case "list_folders":
		return e.execListFolders(ctx, vaultID, arguments)
	case "list_folder_contents":
		return e.execListFolderContents(ctx, vaultID, arguments)
	case "create_document":
		return e.execCreateDocument(ctx, vaultID, arguments)
	case "edit_document":
		return e.execEditDocument(ctx, vaultID, arguments)
	case "edit_document_section":
		return "", nil, &tools.ToolError{
			Message: "edit_document_section is not supported on remote vaults. Use get_document to read the full content, edit locally, then use edit_document to save.",
		}
	case "create_memory":
		return e.execCreateMemory(ctx, vaultID, arguments)
	case "get_document_versions":
		return e.execGetDocumentVersions(ctx, vaultID, arguments)
	default:
		return "", nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (e *Executor) execSearch(ctx context.Context, vaultID, arguments string) (string, *tools.ToolResultMeta, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse search input: %w", err)
	}

	start := time.Now()
	results, err := e.client.SearchDocuments(ctx, vaultID, input.Query, 20)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("remote search: %w", err)
	}

	var sb strings.Builder
	totalChunks := 0
	for _, r := range results {
		fmt.Fprintf(&sb, "## %s (%s)\n", r.Title, r.Path)
		totalChunks += len(r.MatchedChunks)
		for _, ch := range r.MatchedChunks {
			sb.WriteString(ch.Snippet)
			sb.WriteString("\n\n")
		}
	}

	result := sb.String()
	if result == "" {
		result = "No results found."
	}

	resultCount := len(results)
	meta := &tools.ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: &resultCount,
		ChunkCount:  &totalChunks,
	}
	return result, meta, nil
}

func (e *Executor) execReadDocument(ctx context.Context, vaultID, arguments string) (string, *tools.ToolResultMeta, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse read_document input: %w", err)
	}

	start := time.Now()
	doc, err := e.client.GetDocument(ctx, vaultID, input.Path)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		var httpErr *apiclient.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return fmt.Sprintf("Document not found: %s", input.Path), &tools.ToolResultMeta{DurationMs: durationMs}, nil
		}
		return "", nil, fmt.Errorf("remote read document: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", doc.Title)
	if doc.Hash != nil {
		fmt.Fprintf(&sb, "Content-Hash: %s\n\n", *doc.Hash)
	}
	sb.WriteString(doc.Content)

	contentLen := len(doc.Content)
	meta := &tools.ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
		ContentLength: &contentLen,
	}
	return sb.String(), meta, nil
}

func (e *Executor) execListLabels(ctx context.Context, vaultID string) (string, *tools.ToolResultMeta, error) {
	start := time.Now()
	labels, err := e.client.ListLabels(ctx, vaultID)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("remote list labels: %w", err)
	}

	result := "No labels found."
	if len(labels) > 0 {
		result = strings.Join(labels, ", ")
	}
	count := len(labels)
	meta := &tools.ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: &count,
	}
	return result, meta, nil
}

func (e *Executor) execListFolders(ctx context.Context, vaultID, arguments string) (string, *tools.ToolResultMeta, error) {
	var input struct {
		Parent *string `json:"parent"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse list_folders input: %w", err)
	}

	start := time.Now()
	folders, err := e.client.ListFolders(ctx, vaultID, input.Parent)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("remote list folders: %w", err)
	}

	var sb strings.Builder
	for _, f := range folders {
		fmt.Fprintf(&sb, "%s (%s)\n", f.Path, f.Name)
	}
	result := sb.String()
	if result == "" {
		result = "No folders found."
	}
	count := len(folders)
	meta := &tools.ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: &count,
	}
	return result, meta, nil
}

func (e *Executor) execListFolderContents(ctx context.Context, vaultID, arguments string) (string, *tools.ToolResultMeta, error) {
	var input struct {
		Folder string `json:"folder"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse list_folder_contents input: %w", err)
	}
	if input.Folder == "" {
		return "", nil, fmt.Errorf("folder is required")
	}

	start := time.Now()
	entries, err := e.client.ListFiles(ctx, vaultID, input.Folder, false)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("remote list folder contents: %w", err)
	}

	var sb strings.Builder
	count := 0
	for _, entry := range entries {
		if entry.IsDir {
			fmt.Fprintf(&sb, "📁 %s/\n", entry.Name)
		} else {
			fmt.Fprintf(&sb, "📄 %s\n", entry.Path)
		}
		count++
	}

	result := sb.String()
	if result == "" {
		result = fmt.Sprintf("No contents found in folder %s", input.Folder)
	}
	meta := &tools.ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: &count,
	}
	return result, meta, nil
}

func (e *Executor) execCreateDocument(ctx context.Context, vaultID, arguments string) (string, *tools.ToolResultMeta, error) {
	return e.execUpsertDocument(ctx, vaultID, arguments, "create")
}

func (e *Executor) execEditDocument(ctx context.Context, vaultID, arguments string) (string, *tools.ToolResultMeta, error) {
	return e.execUpsertDocument(ctx, vaultID, arguments, "edit")
}

func (e *Executor) execUpsertDocument(ctx context.Context, vaultID, arguments, verb string) (string, *tools.ToolResultMeta, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse %s_document input: %w", verb, err)
	}

	start := time.Now()
	doc, err := e.client.CreateDocument(ctx, apiclient.CreateDocumentRequest{
		VaultName: vaultID,
		Path:      input.Path,
		Content:   input.Content,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("remote %s document: %w", verb, err)
	}

	pastTense := "created"
	if verb == "edit" {
		pastTense = "updated"
	}

	meta := &tools.ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	}
	return fmt.Sprintf("Document %s: %s (%s)", pastTense, doc.Title, doc.Path), meta, nil
}

func (e *Executor) execCreateMemory(ctx context.Context, vaultID, arguments string) (string, *tools.ToolResultMeta, error) {
	var input struct {
		Title   string   `json:"title"`
		Content string   `json:"content"`
		Project string   `json:"project"`
		Labels  []string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse create_memory input: %w", err)
	}

	// Remote executor uses default settings since vault settings aren't available remotely
	path, fullContent := tools.BuildMemoryDocument(input.Project, input.Title, input.Content, input.Labels, models.VaultSettings{})

	start := time.Now()
	doc, err := e.client.CreateDocument(ctx, apiclient.CreateDocumentRequest{
		VaultName: vaultID,
		Path:      path,
		Content:   fullContent,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("remote create memory: %w", err)
	}

	meta := &tools.ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	}
	return fmt.Sprintf("Memory created at %s", doc.Path), meta, nil
}

func (e *Executor) execGetDocumentVersions(ctx context.Context, vaultID, arguments string) (string, *tools.ToolResultMeta, error) {
	var input struct {
		Path  string `json:"path"`
		Limit *int   `json:"limit"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse get_document_versions input: %w", err)
	}

	limit := 20
	if input.Limit != nil && *input.Limit > 0 {
		limit = *input.Limit
	}

	start := time.Now()
	versions, err := e.client.ListVersions(ctx, vaultID, input.Path, limit)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		var httpErr *apiclient.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return fmt.Sprintf("Document not found: %s", input.Path), &tools.ToolResultMeta{DurationMs: durationMs}, nil
		}
		return "", nil, fmt.Errorf("remote get document versions: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Document: %s\n", input.Path)
	fmt.Fprintf(&sb, "Total versions: %d\n\n", len(versions))

	if len(versions) == 0 {
		sb.WriteString("No previous versions.\n")
	} else {
		for _, v := range versions {
			fmt.Fprintf(&sb, "### Version %d\n", v.Version)
			fmt.Fprintf(&sb, "- Title: %s\n", v.Title)
			fmt.Fprintf(&sb, "- Created: %s\n", v.CreatedAt.Format(time.RFC3339))
			fmt.Fprintf(&sb, "- Hash: %s\n\n", v.Hash)
		}
	}

	count := len(versions)
	meta := &tools.ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: &count,
	}
	return sb.String(), meta, nil
}
