// Package webclip provides shared logic for fetching web pages as markdown
// and optionally persisting them to a vault. Used by both the agent tool
// and the REST API handler.
package webclip

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/jina"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/metrics"
	"github.com/raphi011/know/internal/models"
)

// cleanupSystemPrompt instructs the LLM to fix markdown formatting and strip non-content boilerplate.
const cleanupSystemPrompt = `You are a markdown formatting assistant. Clean up the formatting of the following markdown text that was extracted from a web page.

Rules:
- Fix broken headings, lists, links, and code blocks
- Remove navigation elements, cookie banners, and other non-content boilerplate
- Collapse excessive blank lines
- Fix broken tables
- Do NOT change, summarize, or omit any actual content
- Do NOT add commentary or explanations
- Return ONLY the cleaned markdown, nothing else`

// maxCleanupInputLen is the maximum markdown size (in bytes) that CleanMarkdown
// will send to the LLM. Larger inputs risk token-limit truncation or excessive cost.
const maxCleanupInputLen = 100_000 // ~100KB

// CleanMarkdown passes markdown through an LLM to fix formatting issues
// without changing article content. Model must be non-nil.
func CleanMarkdown(ctx context.Context, model *llm.Model, markdown string) (string, error) {
	if model == nil {
		return "", fmt.Errorf("clean markdown: model is required")
	}
	if len(markdown) > maxCleanupInputLen {
		return "", fmt.Errorf("clean markdown: input too large (%d bytes, max %d)", len(markdown), maxCleanupInputLen)
	}

	logger := logutil.FromCtx(ctx)
	logger.Debug("cleaning markdown with LLM", "input_len", len(markdown))

	cleaned, err := model.GenerateWithSystem(ctx, cleanupSystemPrompt, markdown)
	if err != nil {
		return "", fmt.Errorf("clean markdown: %w", err)
	}
	if strings.TrimSpace(cleaned) == "" {
		return "", fmt.Errorf("clean markdown: LLM returned empty output")
	}

	logger.Debug("markdown cleanup complete", "input_len", len(markdown), "output_len", len(cleaned))

	return cleaned, nil
}

// Result holds the outcome of a web fetch, with optional save path.
type Result struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Markdown string `json:"markdown,omitempty"`
	Path     string `json:"path,omitempty"` // vault path (empty if not saved)
}

// Fetch fetches a URL via Jina Reader and returns the result without persisting.
// If model is non-nil, the markdown is cleaned up via LLM before returning.
func Fetch(ctx context.Context, client *jina.Client, url string, model *llm.Model, m *metrics.Metrics) (_ *Result, err error) {
	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
		}
		m.RecordWebClip(model != nil, status, time.Since(start))
	}()

	r, err := client.Read(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	markdown := r.Markdown
	if model != nil {
		markdown, err = CleanMarkdown(ctx, model, markdown)
		if err != nil {
			return nil, fmt.Errorf("clean: %w", err)
		}
	}

	return &Result{
		Title:    r.Title,
		URL:      r.URL,
		Markdown: markdown,
	}, nil
}

// FormatAsMarkdown formats a Result as a markdown string with title and source header.
func FormatAsMarkdown(r *Result) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n", r.Title)
	fmt.Fprintf(&sb, "Source: %s\n\n", r.URL)
	sb.WriteString(r.Markdown)
	return sb.String()
}

// FetchAndSave fetches a URL, builds frontmatter, and persists the document
// to the vault via FileService.Create. If customPath is nil or empty, the path
// is derived from the page title and the vault's WebClipPath setting.
// If model is non-nil, the markdown is cleaned up via LLM before saving.
func FetchAndSave(ctx context.Context, client *jina.Client, fileSvc *file.Service, vaultID, url string, customPath *string, settings models.VaultSettings, model *llm.Model, m *metrics.Metrics) (_ *Result, err error) {
	start := time.Now()
	defer func() {
		status := "success"
		if err != nil {
			status = "error"
		}
		m.RecordWebClip(model != nil, status, time.Since(start))
	}()

	r, err := client.Read(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	if model != nil {
		r.Markdown, err = CleanMarkdown(ctx, model, r.Markdown)
		if err != nil {
			return nil, fmt.Errorf("clean: %w", err)
		}
	}

	// Derive save path.
	savePath := ""
	if customPath != nil {
		savePath = *customPath
	}
	if savePath == "" {
		folder := settings.WebClipPath
		if folder == "" {
			folder = models.DefaultWebClipPath
		}
		folder = strings.TrimSuffix(folder, "/")
		slug := slugify(r.Title)
		if slug == "" {
			slug = slugify(url)
		}
		if slug == "" {
			slug = "untitled"
		}
		savePath = folder + "/" + slug + ".md"
	}

	// Build frontmatter + content.
	content := buildContent(r, url)

	_, err = fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    savePath,
		Content: content,
	})
	if err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}

	return &Result{
		Title:    r.Title,
		URL:      r.URL,
		Markdown: r.Markdown,
		Path:     savePath,
	}, nil
}

// buildContent prepends YAML frontmatter to the fetched markdown.
func buildContent(r *jina.ReadResult, sourceURL string) string {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("title: %q\n", r.Title))
	sb.WriteString(fmt.Sprintf("source: %q\n", sourceURL))
	if r.URL != "" && r.URL != sourceURL {
		sb.WriteString(fmt.Sprintf("canonical_url: %q\n", r.URL))
	}
	if r.Description != "" {
		sb.WriteString(fmt.Sprintf("description: %q\n", r.Description))
	}
	sb.WriteString(fmt.Sprintf("fetched_at: %q\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString("labels:\n  - web-clip\n")
	sb.WriteString("---\n\n")
	sb.WriteString(r.Markdown)

	return sb.String()
}

// multiHyphen collapses consecutive hyphens.
var multiHyphen = regexp.MustCompile(`-{2,}`)

// slugify converts a string to a URL-safe ASCII slug.
func slugify(s string) string {
	// Strip protocol prefix for URLs.
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")

	// Map to lowercase ASCII letters/digits, everything else becomes a hyphen.
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return '-'
	}, s)

	s = multiHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if len(s) > 80 {
		s = s[:80]
		s = strings.TrimRight(s, "-")
	}

	return s
}
