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
	"github.com/raphi011/know/internal/models"
)

// Result holds the outcome of a web fetch, with optional save path.
type Result struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	Markdown string `json:"markdown,omitempty"`
	Path     string `json:"path,omitempty"` // vault path (empty if not saved)
}

// Fetch fetches a URL via Jina Reader and returns the result without persisting.
func Fetch(ctx context.Context, client *jina.Client, url string) (*Result, error) {
	r, err := client.Read(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	return &Result{
		Title:    r.Title,
		URL:      r.URL,
		Markdown: r.Markdown,
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
func FetchAndSave(ctx context.Context, client *jina.Client, fileSvc *file.Service, vaultID, url string, customPath *string, settings models.VaultSettings) (*Result, error) {
	r, err := client.Read(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
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
