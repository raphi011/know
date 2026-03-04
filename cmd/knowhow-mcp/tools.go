package main

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/raphi011/knowhow/internal/graphqlclient"
	"golang.org/x/sync/errgroup"
)

// ---------- GraphQL response types ----------

type meResponse struct {
	Me struct {
		VaultAccess []string `json:"vaultAccess"`
	} `json:"me"`
}

type vaultsResponse struct {
	Vaults []struct {
		ID     string   `json:"id"`
		Name   string   `json:"name"`
		Labels []string `json:"labels"`
	} `json:"vaults"`
}

type searchResponse struct {
	Search []struct {
		DocumentID    string   `json:"documentId"`
		Path          string   `json:"path"`
		Title         string   `json:"title"`
		Labels        []string `json:"labels"`
		DocType       *string  `json:"docType"`
		Score         float64  `json:"score"`
		MatchedChunks []struct {
			Snippet     string  `json:"snippet"`
			HeadingPath *string `json:"headingPath"`
			Score       float64 `json:"score"`
		} `json:"matchedChunks"`
	} `json:"search"`
}

type documentResponse struct {
	Document *struct {
		Title   string   `json:"title"`
		Path    string   `json:"path"`
		Content string   `json:"content"`
		Labels  []string `json:"labels"`
		DocType *string  `json:"docType"`
		Source  string   `json:"source"`
	} `json:"document"`
}

type foldersResponse struct {
	Vaults []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Folders []struct {
			Path string `json:"path"`
			Name string `json:"name"`
		} `json:"folders"`
	} `json:"vaults"`
}

type createDocumentResponse struct {
	CreateDocument struct {
		Path string `json:"path"`
	} `json:"createDocument"`
}

// ---------- Tool input types ----------

type SearchInput struct {
	Query    string   `json:"query" jsonschema:"description=Search query text"`
	Labels   []string `json:"labels,omitempty" jsonschema:"description=Filter by labels"`
	DocType  *string  `json:"doc_type,omitempty" jsonschema:"description=Filter by document type"`
	Folder   *string  `json:"folder,omitempty" jsonschema:"description=Filter by folder path prefix"`
	Limit    *int     `json:"limit,omitempty" jsonschema:"description=Max results (default 20)"`
	Instance *string  `json:"instance,omitempty" jsonschema:"description=Instance name (searches all if omitted)"`
}

type GetDocumentInput struct {
	Path     string  `json:"path" jsonschema:"description=Document path"`
	Instance *string `json:"instance,omitempty" jsonschema:"description=Instance name (tries all if omitted)"`
}

type ListLabelsInput struct {
	Instance *string `json:"instance,omitempty" jsonschema:"description=Instance name (lists all if omitted)"`
}

type ListFoldersInput struct {
	Instance *string `json:"instance,omitempty" jsonschema:"description=Instance name (lists all if omitted)"`
}

type CreateMemoryInput struct {
	Title    string   `json:"title" jsonschema:"description=Memory title"`
	Content  string   `json:"content" jsonschema:"description=Memory content (markdown)"`
	Labels   []string `json:"labels,omitempty" jsonschema:"description=Additional labels (memory label is always added)"`
	Instance string   `json:"instance" jsonschema:"description=Target instance name"`
}

// ---------- connectedInstance ----------

type connectedInstance struct {
	name     string
	client   *graphqlclient.Client
	vaultIDs []string

	mu sync.Mutex
}

// resolveVaults lazily fetches vault IDs from the instance. On success the
// result is cached for the server's lifetime; on failure it retries on the
// next call so that transient errors don't permanently disable an instance.
func (ci *connectedInstance) resolveVaults(ctx context.Context) error {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	if ci.vaultIDs != nil {
		return nil
	}

	var resp meResponse
	if err := ci.client.Do(ctx, `query { me { vaultAccess } }`, nil, &resp); err != nil {
		return fmt.Errorf("resolve vaults for %q: %w", ci.name, err)
	}
	if len(resp.Me.VaultAccess) == 0 {
		return fmt.Errorf("no vault access for instance %q", ci.name)
	}
	ci.vaultIDs = resp.Me.VaultAccess
	return nil
}

// ---------- tool registrar ----------

type mcpTools struct {
	instances []*connectedInstance
}

func newMCPTools(cfg *Config) *mcpTools {
	instances := make([]*connectedInstance, len(cfg.Instances))
	for i, inst := range cfg.Instances {
		instances[i] = &connectedInstance{
			name:   inst.Name,
			client: newGQLClient(inst),
		}
	}
	return &mcpTools{instances: instances}
}

func (t *mcpTools) filterInstances(name *string) []*connectedInstance {
	if name == nil {
		return t.instances
	}
	for _, inst := range t.instances {
		if inst.name == *name {
			return []*connectedInstance{inst}
		}
	}
	return nil
}

func (t *mcpTools) register(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_documents",
		Description: "Search documents across knowhow instances using full-text and semantic search. Returns titles, paths, scores, and matching snippets.",
	}, t.searchDocuments)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_document",
		Description: "Get a document by its path. Returns the full content, title, labels, and metadata.",
	}, t.getDocument)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_labels",
		Description: "List all labels used across documents. Useful for discovering available categories before searching.",
	}, t.listLabels)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_folders",
		Description: "List the folder structure of a knowhow vault. Use to browse and understand vault organization before searching.",
	}, t.listFolders)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_memory",
		Description: "Create a new memory document. Memories are short notes stored under /memories/ with a date-prefixed path. Always adds the 'memory' label.",
	}, t.createMemory)
}

// ---------- Tool handlers ----------

func (t *mcpTools) searchDocuments(ctx context.Context, req *mcp.CallToolRequest, input SearchInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Query) == "" {
		return nil, nil, fmt.Errorf("query is required")
	}
	if input.Limit != nil && *input.Limit < 1 {
		return nil, nil, fmt.Errorf("limit must be positive")
	}

	instances := t.filterInstances(input.Instance)
	if input.Instance != nil && len(instances) == 0 {
		return nil, nil, fmt.Errorf("unknown instance %q", *input.Instance)
	}

	// Collect (instance, vault) pairs for concurrent fan-out.
	type searchJob struct {
		inst    *connectedInstance
		vaultID string
	}
	var jobs []searchJob
	var resolveErrors []string
	for _, inst := range instances {
		if err := inst.resolveVaults(ctx); err != nil {
			resolveErrors = append(resolveErrors, fmt.Sprintf("## %s\nError: %v\n\n", inst.name, err))
			continue
		}
		for _, vaultID := range inst.vaultIDs {
			jobs = append(jobs, searchJob{inst: inst, vaultID: vaultID})
		}
	}

	// Fan out search queries concurrently. Each goroutine writes to its own
	// slot in results; g.Wait() synchronizes before reading.
	results := make([]string, len(jobs))
	var g errgroup.Group
	for i, job := range jobs {
		g.Go(func() error {
			vars := map[string]any{
				"input": map[string]any{
					"vaultId": job.vaultID,
					"query":   input.Query,
					"labels":  input.Labels,
					"docType": input.DocType,
					"folder":  input.Folder,
					"limit":   input.Limit,
				},
			}
			var resp searchResponse
			if err := job.inst.client.Do(ctx, `query Search($input: SearchInput!) { search(input: $input) { documentId path title labels docType score matchedChunks { snippet headingPath score } } }`, vars, &resp); err != nil {
				slog.Warn("search query failed", "instance", job.inst.name, "vault", job.vaultID, "error", err)
				results[i] = fmt.Sprintf("## %s (vault %s)\nError: %v\n\n", job.inst.name, job.vaultID, err)
				return nil
			}
			if len(resp.Search) == 0 {
				return nil
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "## %s\n", job.inst.name)
			for _, r := range resp.Search {
				fmt.Fprintf(&sb, "### %s\n- Path: %s\n- Score: %.3f\n- Labels: %s\n", r.Title, r.Path, r.Score, strings.Join(r.Labels, ", "))
				for _, ch := range r.MatchedChunks {
					fmt.Fprintf(&sb, "  > %s\n", ch.Snippet)
				}
				sb.WriteString("\n")
			}
			results[i] = sb.String()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		slog.Warn("unexpected errgroup error", "error", err)
	}

	var sb strings.Builder
	for _, e := range resolveErrors {
		sb.WriteString(e)
	}
	for _, r := range results {
		sb.WriteString(r)
	}

	if sb.Len() == 0 {
		return textResult("No results found."), nil, nil
	}
	return textResult(sb.String()), nil, nil
}

func (t *mcpTools) getDocument(ctx context.Context, req *mcp.CallToolRequest, input GetDocumentInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Path) == "" {
		return nil, nil, fmt.Errorf("path is required")
	}

	instances := t.filterInstances(input.Instance)
	if input.Instance != nil && len(instances) == 0 {
		return nil, nil, fmt.Errorf("unknown instance %q", *input.Instance)
	}

	var errs []string
	for _, inst := range instances {
		if err := inst.resolveVaults(ctx); err != nil {
			slog.Warn("vault resolution failed", "instance", inst.name, "error", err)
			errs = append(errs, fmt.Sprintf("%s: %v", inst.name, err))
			continue
		}
		for _, vaultID := range inst.vaultIDs {
			vars := map[string]any{
				"vaultId": vaultID,
				"path":    input.Path,
			}
			var resp documentResponse
			if err := inst.client.Do(ctx, `query GetDoc($vaultId: ID!, $path: String!) { document(vaultId: $vaultId, path: $path) { title path content labels docType source } }`, vars, &resp); err != nil {
				slog.Warn("get document failed", "instance", inst.name, "vault", vaultID, "path", input.Path, "error", err)
				errs = append(errs, fmt.Sprintf("%s: %v", inst.name, err))
				continue
			}
			if resp.Document == nil {
				continue
			}
			d := resp.Document
			var sb strings.Builder
			fmt.Fprintf(&sb, "# %s\n\n", d.Title)
			fmt.Fprintf(&sb, "- Instance: %s\n", inst.name)
			fmt.Fprintf(&sb, "- Path: %s\n", d.Path)
			fmt.Fprintf(&sb, "- Labels: %s\n", strings.Join(d.Labels, ", "))
			if d.DocType != nil {
				fmt.Fprintf(&sb, "- Type: %s\n", *d.DocType)
			}
			fmt.Fprintf(&sb, "\n---\n\n%s", d.Content)
			return textResult(sb.String()), nil, nil
		}
	}

	if len(errs) > 0 {
		return textResult(fmt.Sprintf("Document not found: %s\n\nErrors:\n- %s", input.Path, strings.Join(errs, "\n- "))), nil, nil
	}
	return textResult(fmt.Sprintf("Document not found: %s", input.Path)), nil, nil
}

func (t *mcpTools) listLabels(ctx context.Context, req *mcp.CallToolRequest, input ListLabelsInput) (*mcp.CallToolResult, any, error) {
	instances := t.filterInstances(input.Instance)
	if input.Instance != nil && len(instances) == 0 {
		return nil, nil, fmt.Errorf("unknown instance %q", *input.Instance)
	}

	var sb strings.Builder
	for _, inst := range instances {
		var resp vaultsResponse
		if err := inst.client.Do(ctx, `query { vaults { id name labels } }`, nil, &resp); err != nil {
			slog.Warn("list labels failed", "instance", inst.name, "error", err)
			fmt.Fprintf(&sb, "## %s\nError: %v\n\n", inst.name, err)
			continue
		}

		// Deduplicate labels across vaults within the instance
		labelSet := map[string]bool{}
		for _, v := range resp.Vaults {
			for _, l := range v.Labels {
				labelSet[l] = true
			}
		}

		labels := make([]string, 0, len(labelSet))
		for l := range labelSet {
			labels = append(labels, l)
		}
		sort.Strings(labels)

		if len(labels) > 0 {
			fmt.Fprintf(&sb, "## %s\n%s\n\n", inst.name, strings.Join(labels, ", "))
		}
	}

	if sb.Len() == 0 {
		return textResult("No labels found."), nil, nil
	}
	return textResult(sb.String()), nil, nil
}

func (t *mcpTools) listFolders(ctx context.Context, req *mcp.CallToolRequest, input ListFoldersInput) (*mcp.CallToolResult, any, error) {
	instances := t.filterInstances(input.Instance)
	if input.Instance != nil && len(instances) == 0 {
		return nil, nil, fmt.Errorf("unknown instance %q", *input.Instance)
	}

	var sb strings.Builder
	for _, inst := range instances {
		var resp foldersResponse
		if err := inst.client.Do(ctx, `query { vaults { id name folders { path name } } }`, nil, &resp); err != nil {
			slog.Warn("list folders failed", "instance", inst.name, "error", err)
			fmt.Fprintf(&sb, "## %s\nError: %v\n\n", inst.name, err)
			continue
		}

		for _, v := range resp.Vaults {
			if len(v.Folders) == 0 {
				continue
			}
			fmt.Fprintf(&sb, "## %s / %s\n", inst.name, v.Name)
			sb.WriteString(buildFolderTree(v.Folders))
			sb.WriteString("\n")
		}
	}

	if sb.Len() == 0 {
		return textResult("No folders found."), nil, nil
	}
	return textResult(sb.String()), nil, nil
}

// buildFolderTree formats a flat list of folder paths into an indented tree display.
func buildFolderTree(folders []struct {
	Path string `json:"path"`
	Name string `json:"name"`
}) string {
	if len(folders) == 0 {
		return ""
	}

	// Sort paths for consistent display
	sort.Slice(folders, func(i, j int) bool {
		return folders[i].Path < folders[j].Path
	})

	var sb strings.Builder
	for _, f := range folders {
		depth := strings.Count(strings.Trim(f.Path, "/"), "/")
		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(&sb, "%s%s/\n", indent, f.Name)
	}
	return sb.String()
}

func (t *mcpTools) createMemory(ctx context.Context, req *mcp.CallToolRequest, input CreateMemoryInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(input.Title) == "" {
		return nil, nil, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, nil, fmt.Errorf("content is required")
	}

	instances := t.filterInstances(&input.Instance)
	if len(instances) == 0 {
		return nil, nil, fmt.Errorf("unknown instance %q", input.Instance)
	}
	inst := instances[0]

	if err := inst.resolveVaults(ctx); err != nil {
		return nil, nil, fmt.Errorf("resolve vaults: %w", err)
	}

	// Build labels list — always include "memory"
	labels := []string{"memory"}
	for _, l := range input.Labels {
		if l != "memory" {
			labels = append(labels, l)
		}
	}

	// Build frontmatter with YAML-safe label values
	var content strings.Builder
	content.WriteString("---\nlabels:\n")
	for _, l := range labels {
		fmt.Fprintf(&content, "  - %q\n", l)
	}
	content.WriteString("---\n\n")
	content.WriteString(input.Content)

	// Generate path
	slug := slugify(input.Title)
	date := time.Now().Format("2006-01-02")
	path := fmt.Sprintf("/memories/%s-%s.md", date, slug)

	// Create in the first accessible vault
	vaultID := inst.vaultIDs[0]
	vars := map[string]any{
		"vaultId": vaultID,
		"file": map[string]any{
			"path":    path,
			"content": content.String(),
		},
		"source": "mcp",
	}

	var resp createDocumentResponse
	if err := inst.client.Do(ctx, `mutation CreateDoc($vaultId: ID!, $file: FileInput!, $source: String) { createDocument(vaultId: $vaultId, file: $file, source: $source) { path } }`, vars, &resp); err != nil {
		slog.Warn("create memory failed", "instance", inst.name, "vault", vaultID, "error", err)
		return nil, nil, fmt.Errorf("create memory: %w", err)
	}

	return textResult(fmt.Sprintf("Memory created at %s (instance: %s, vault: %s)", resp.CreateDocument.Path, inst.name, vaultID)), nil, nil
}

// ---------- helpers ----------

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}
