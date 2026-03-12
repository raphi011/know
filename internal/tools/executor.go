package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/memory"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/parser"
	"github.com/raphi011/knowhow/internal/pathutil"
	"github.com/raphi011/knowhow/internal/search"
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
}

// ExecuteTool runs a tool by canonical name with JSON-encoded arguments
// scoped to a single vault. Returns the result text, optional metadata,
// and an error.
func (e *Executor) ExecuteTool(ctx context.Context, vaultID, toolName, arguments string) (string, *ToolResultMeta, error) {
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
		return e.execEditDocumentSection(ctx, vaultID, arguments)
	case "create_memory":
		return e.execCreateMemory(ctx, vaultID, arguments)
	case "get_document_versions":
		return e.execGetDocumentVersions(ctx, vaultID, arguments)
	default:
		return "", nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (e *Executor) execSearch(ctx context.Context, vaultID, arguments string) (string, *ToolResultMeta, error) {
	var input struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse search input: %w", err)
	}

	start := time.Now()
	results, err := e.Search.Search(ctx, search.SearchInput{
		VaultID:     vaultID,
		Query:       input.Query,
		Limit:       20,
		FullContent: true,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("search: %w", err)
	}

	var sb strings.Builder
	var matchedDocs []ToolDocRef
	totalChunks := 0
	for _, r := range results {
		fmt.Fprintf(&sb, "## %s (%s)\n", r.Title, r.Path)
		matchedDocs = append(matchedDocs, ToolDocRef{Title: r.Title, Path: r.Path, Score: r.Score})
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

	meta := &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(len(results)),
		ChunkCount:  new(totalChunks),
		MatchedDocs: matchedDocs,
	}
	return result, meta, nil
}

func (e *Executor) execReadDocument(ctx context.Context, vaultID, arguments string) (string, *ToolResultMeta, error) {
	var input struct {
		Path     string `json:"path"`
		Sections bool   `json:"sections"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse read_document input: %w", err)
	}

	start := time.Now()
	doc, err := e.DB.GetDocumentByPath(ctx, vaultID, input.Path)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("read document: %w", err)
	}
	if doc == nil {
		meta := &ToolResultMeta{DurationMs: durationMs}
		return fmt.Sprintf("Document not found: %s", input.Path), meta, nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", doc.Title)

	if doc.ContentHash != nil {
		fmt.Fprintf(&sb, "Content-Hash: %s\n\n", *doc.ContentHash)
	}

	if input.Sections {
		parsed, parseErr := parser.ParseMarkdown(doc.ContentBody)
		if parseErr != nil {
			logutil.FromCtx(ctx).Warn("parse markdown for section outline", "path", input.Path, "error", parseErr)
			fmt.Fprintf(&sb, "**Warning**: could not parse sections: %v\n\n", parseErr)
		} else if len(parsed.Sections) > 0 {
			outline := parser.SectionOutline(parsed)
			sb.WriteString("## Sections\n")
			sb.WriteString("| # | Heading | Pos |\n")
			sb.WriteString("|---|---------|-----|\n")
			for _, info := range outline {
				heading := info.Heading
				if heading == "" {
					heading = "(preamble)"
				}
				fmt.Fprintf(&sb, "| %d | %s | %d |\n", info.Index, heading, info.Position)
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString(doc.ContentBody)

	contentLen := len(doc.ContentBody)
	meta := &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
		ContentLength: &contentLen,
	}
	return sb.String(), meta, nil
}

func (e *Executor) execListLabels(ctx context.Context, vaultID string) (string, *ToolResultMeta, error) {
	start := time.Now()
	labels, err := e.DB.ListLabels(ctx, vaultID)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("list labels: %w", err)
	}

	result := "No labels found."
	if len(labels) > 0 {
		result = strings.Join(labels, ", ")
	}
	meta := &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(len(labels)),
	}
	return result, meta, nil
}

func (e *Executor) execListFolders(ctx context.Context, vaultID, arguments string) (string, *ToolResultMeta, error) {
	var input struct {
		Parent *string `json:"parent"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse list_folders input: %w", err)
	}

	start := time.Now()
	folders, err := e.DB.ListFolders(ctx, vaultID)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("list folders: %w", err)
	}

	if input.Parent != nil {
		parent := pathutil.NormalizeFolderPath(*input.Parent)
		var filtered []models.Folder
		for _, f := range folders {
			if pathutil.IsImmediateChildFolder(parent, f.Path) {
				filtered = append(filtered, f)
			}
		}
		folders = filtered
	}

	var sb strings.Builder
	for _, f := range folders {
		fmt.Fprintf(&sb, "%s (%s)\n", f.Path, f.Name)
	}
	result := sb.String()
	if result == "" {
		result = "No folders found."
	}
	meta := &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(len(folders)),
	}
	return result, meta, nil
}

func (e *Executor) execListFolderContents(ctx context.Context, vaultID, arguments string) (string, *ToolResultMeta, error) {
	var input struct {
		Folder string `json:"folder"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse list_folder_contents input: %w", err)
	}
	if input.Folder == "" {
		return "", nil, fmt.Errorf("folder is required")
	}
	folder := pathutil.NormalizeFolderPath(input.Folder)

	start := time.Now()

	docs, err := e.DB.ListDocuments(ctx, db.ListDocumentsFilter{
		VaultID: vaultID,
		Folder:  &folder,
	})
	if err != nil {
		return "", nil, fmt.Errorf("list folder contents: %w", err)
	}
	allFolders, err := e.DB.ListFolders(ctx, vaultID)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("list folder subfolders: %w", err)
	}

	var sb strings.Builder
	count := 0

	for _, f := range allFolders {
		if pathutil.IsImmediateChildFolder(folder, f.Path) {
			fmt.Fprintf(&sb, "📁 %s/\n", f.Name)
			count++
		}
	}

	for _, d := range docs {
		if !pathutil.IsImmediateChild(folder, d.Path) {
			continue
		}
		labels := ""
		if len(d.Labels) > 0 {
			labels = " [" + strings.Join(d.Labels, ", ") + "]"
		}
		fmt.Fprintf(&sb, "📄 %s — %s%s\n", d.Path, d.Title, labels)
		count++
	}

	result := sb.String()
	if result == "" {
		result = fmt.Sprintf("No contents found in folder %s", folder)
	}
	meta := &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(count),
	}
	return result, meta, nil
}

func (e *Executor) execCreateDocument(ctx context.Context, vaultID, arguments string) (string, *ToolResultMeta, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", nil, fmt.Errorf("parse create_document input: %w", err)
	}
	if args.Path == "" {
		return "", nil, fmt.Errorf("path is required")
	}
	if args.Content == "" {
		return "", nil, fmt.Errorf("content is required")
	}

	existing, err := e.DB.GetDocumentByPath(ctx, vaultID, args.Path)
	if err != nil {
		return "", nil, fmt.Errorf("check existing document: %w", err)
	}
	if existing != nil {
		return "", nil, &ToolError{Message: fmt.Sprintf("document already exists at path: %s", args.Path)}
	}

	start := time.Now()
	doc, err := e.DocService.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    args.Path,
		Content: args.Content,
		Source:  models.SourceAIGenerated,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("create document: %w", err)
	}

	meta := &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	}
	return fmt.Sprintf("Document created: %s (%s)", doc.Title, doc.Path), meta, nil
}

func (e *Executor) execEditDocument(ctx context.Context, vaultID, arguments string) (string, *ToolResultMeta, error) {
	var args struct {
		Path         string  `json:"path"`
		Content      string  `json:"content"`
		ExpectedHash *string `json:"expected_hash"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", nil, fmt.Errorf("parse edit_document input: %w", err)
	}
	if args.Path == "" {
		return "", nil, fmt.Errorf("path is required")
	}
	if args.Content == "" {
		return "", nil, fmt.Errorf("content is required")
	}

	existing, err := e.DB.GetDocumentByPath(ctx, vaultID, args.Path)
	if err != nil {
		return "", nil, fmt.Errorf("check document: %w", err)
	}
	if existing == nil {
		return "", nil, &ToolError{Message: fmt.Sprintf("document not found: %s", args.Path)}
	}
	if err := checkContentHash(args.ExpectedHash, existing.ContentHash); err != nil {
		return "", nil, err
	}

	start := time.Now()
	doc, err := e.DocService.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    args.Path,
		Content: args.Content,
		Source:  models.SourceAIGenerated,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("edit document: %w", err)
	}

	meta := &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	}
	return fmt.Sprintf("Document updated: %s (%s)", doc.Title, doc.Path), meta, nil
}

func (e *Executor) execEditDocumentSection(ctx context.Context, vaultID, arguments string) (string, *ToolResultMeta, error) {
	var args struct {
		Path         string  `json:"path"`
		Operation    string  `json:"operation"`
		Heading      *string `json:"heading"`
		Position     *int    `json:"position"`
		Content      *string `json:"content"`
		NewHeading   *string `json:"new_heading"`
		NewLevel     *int    `json:"new_level"`
		ExpectedHash *string `json:"expected_hash"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", nil, fmt.Errorf("parse edit_document_section input: %w", err)
	}
	if args.Path == "" {
		return "", nil, fmt.Errorf("path is required")
	}
	if args.Operation == "" {
		return "", nil, fmt.Errorf("operation is required")
	}
	// Validate operation early before DB lookup
	switch parser.SectionOperation(args.Operation) {
	case parser.OpReplace, parser.OpInsertAfter, parser.OpInsertBefore, parser.OpDelete, parser.OpAppend:
		// valid
	default:
		return "", nil, &ToolError{Message: fmt.Sprintf("unknown operation: %s", args.Operation)}
	}

	existing, err := e.DB.GetDocumentByPath(ctx, vaultID, args.Path)
	if err != nil {
		return "", nil, fmt.Errorf("check document: %w", err)
	}
	if existing == nil {
		return "", nil, &ToolError{Message: fmt.Sprintf("document not found: %s", args.Path)}
	}
	if err := checkContentHash(args.ExpectedHash, existing.ContentHash); err != nil {
		return "", nil, err
	}

	// Build the section edit
	edit := parser.BuildSectionEdit(parser.SectionEditArgs{
		Operation:  args.Operation,
		Heading:    args.Heading,
		Position:   args.Position,
		Content:    args.Content,
		NewHeading: args.NewHeading,
		NewLevel:   args.NewLevel,
	})

	// Apply the section edit to the existing content
	newContent, err := parser.ApplySectionEdit(existing.Content, edit)
	if err != nil {
		return "", nil, &ToolError{Message: fmt.Sprintf("apply section edit: %s", err)}
	}

	start := time.Now()
	doc, err := e.DocService.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    args.Path,
		Content: newContent,
		Source:  models.SourceAIGenerated,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("edit document section: %w", err)
	}

	opDesc := string(args.Operation)
	headingDesc := ""
	if args.Heading != nil {
		headingDesc = *args.Heading
		if headingDesc == "" {
			headingDesc = "(preamble)"
		}
	}
	if args.NewHeading != nil && (args.Operation == "insert_after" || args.Operation == "insert_before" || args.Operation == "append") {
		headingDesc = *args.NewHeading
	}

	meta := &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	}
	return fmt.Sprintf("Section %s: %q in %s (%s)", opDesc, headingDesc, doc.Title, doc.Path), meta, nil
}

// BuildMemoryDocument delegates to memory.BuildMemoryDocument.
// Kept as an alias to avoid breaking remote.Executor imports.
func BuildMemoryDocument(project, title, content string, labels []string, settings models.VaultSettings) (path, fullContent string) {
	return memory.BuildMemoryDocument(project, title, content, labels, settings)
}

func (e *Executor) execCreateMemory(ctx context.Context, vaultID, arguments string) (string, *ToolResultMeta, error) {
	var input struct {
		Title   string   `json:"title"`
		Content string   `json:"content"`
		Project string   `json:"project"`
		Labels  []string `json:"labels"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse create_memory input: %w", err)
	}
	if strings.TrimSpace(input.Title) == "" {
		return "", nil, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return "", nil, fmt.Errorf("content is required")
	}

	// Load vault settings for memory path config
	vault, err := e.DB.GetVault(ctx, vaultID)
	if err != nil {
		return "", nil, fmt.Errorf("create memory: load vault: %w", err)
	}
	if vault == nil {
		return "", nil, fmt.Errorf("create memory: vault not found: %s", vaultID)
	}
	settings := vault.MemoryDefaults()

	path, fullContent := BuildMemoryDocument(input.Project, input.Title, input.Content, input.Labels, settings)

	start := time.Now()
	doc, err := e.DocService.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    path,
		Content: fullContent,
		Source:  models.SourceMCP,
	})
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("create memory: %w", err)
	}

	meta := &ToolResultMeta{
		DurationMs:    durationMs,
		DocumentPath:  &doc.Path,
		DocumentTitle: &doc.Title,
	}
	return fmt.Sprintf("Memory created at %s", doc.Path), meta, nil
}

func (e *Executor) execGetDocumentVersions(ctx context.Context, vaultID, arguments string) (string, *ToolResultMeta, error) {
	var input struct {
		Path  string `json:"path"`
		Limit *int   `json:"limit"`
	}
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", nil, fmt.Errorf("parse get_document_versions input: %w", err)
	}
	if strings.TrimSpace(input.Path) == "" {
		return "", nil, fmt.Errorf("path is required")
	}
	if input.Limit != nil && *input.Limit < 1 {
		return "", nil, fmt.Errorf("limit must be positive")
	}

	limit := 20
	if input.Limit != nil {
		limit = *input.Limit
	}

	doc, err := e.DB.GetDocumentByPath(ctx, vaultID, input.Path)
	if err != nil {
		return "", nil, fmt.Errorf("get document for versions: %w", err)
	}
	if doc == nil {
		return fmt.Sprintf("Document not found: %s", input.Path), nil, nil
	}

	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return "", nil, fmt.Errorf("extract document ID: %w", err)
	}

	start := time.Now()
	versions, err := e.DB.ListVersions(ctx, docID, limit, 0)
	durationMs := time.Since(start).Milliseconds()
	if err != nil {
		return "", nil, fmt.Errorf("list versions: %w", err)
	}

	// Only query total count if we hit the limit (there may be more)
	totalCount := len(versions)
	if len(versions) == limit {
		totalCount, err = e.DB.CountVersions(ctx, docID)
		if err != nil {
			return "", nil, fmt.Errorf("count versions: %w", err)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Document: %s (%s)\n", input.Path, doc.Title)
	fmt.Fprintf(&sb, "Total versions: %d\n\n", totalCount)

	if len(versions) == 0 {
		sb.WriteString("No previous versions.\n")
	} else {
		for _, v := range versions {
			fmt.Fprintf(&sb, "### Version %d\n", v.Version)
			fmt.Fprintf(&sb, "- Title: %s\n", v.Title)
			fmt.Fprintf(&sb, "- Created: %s\n", v.CreatedAt.Format(time.RFC3339))
			fmt.Fprintf(&sb, "- Source: %s\n", v.Source)
			fmt.Fprintf(&sb, "- Hash: %s\n\n", v.ContentHash)
		}
	}

	meta := &ToolResultMeta{
		DurationMs:  durationMs,
		ResultCount: new(totalCount),
	}
	return sb.String(), meta, nil
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
