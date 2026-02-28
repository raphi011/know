package document

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/models"

	"github.com/raphaelgruber/memcp-go/internal/llm"
	"github.com/raphaelgruber/memcp-go/internal/parser"
)

// Service manages document lifecycle: parse → extract → store → link → embed.
type Service struct {
	db       *db.Client
	embedder *llm.Embedder // optional — nil disables embedding
	resolver *LinkResolver
}

// NewService creates a new document service.
func NewService(db *db.Client, embedder *llm.Embedder) *Service {
	return &Service{
		db:       db,
		embedder: embedder,
		resolver: NewLinkResolver(db),
	}
}

// Create runs the full document lifecycle pipeline.
func (s *Service) Create(ctx context.Context, input models.DocumentInput) (*models.Document, error) {
	// 1. Parse frontmatter
	parsed, err := parser.ParseMarkdown(input.Content)
	if err != nil {
		return nil, fmt.Errorf("parse markdown: %w", err)
	}

	// 2. Extract title (frontmatter > h1 > filename)
	title := parsed.Title
	if title == "" {
		title = filenameTitle(input.Path)
	}

	// 3. Extract labels: frontmatter + inline #tags
	fmLabels := parsed.GetFrontmatterStringSlice("labels")
	if fmLabels == nil {
		fmLabels = parsed.GetFrontmatterStringSlice("tags")
	}
	allLabels := ExtractInlineLabels(parsed.Content, append(fmLabels, input.Labels...))

	// 4. Extract doc_type from frontmatter if not set
	docType := input.DocType
	if docType == nil {
		if dt := parsed.GetFrontmatterString("type"); dt != "" {
			docType = &dt
		}
	}

	// 5. Extract metadata from frontmatter
	metadata := input.Metadata
	if metadata == nil {
		metadata = extractMetadata(parsed.Frontmatter)
	}

	// 6. Compute content_body (without frontmatter) and content_hash
	contentBody := parsed.Content
	contentHash := models.ContentHash(input.Content)

	// 7. Normalize path
	path := models.NormalizePath(input.Path)

	// 8. Store document
	dbInput := models.DocumentInput{
		VaultID:     input.VaultID,
		Path:        path,
		Title:       title,
		Content:     input.Content,
		ContentBody: contentBody,
		Source:      input.Source,
		SourcePath:  input.SourcePath,
		ContentHash: &contentHash,
		Labels:      allLabels,
		DocType:     docType,
		Metadata:    metadata,
	}

	doc, created, err := s.db.UpsertDocument(ctx, dbInput)
	if err != nil {
		return nil, fmt.Errorf("upsert document: %w", err)
	}

	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("extract doc id: %w", err)
	}

	// 9. Extract and store wiki-links
	if err := s.processWikiLinks(ctx, docID, input.VaultID, parsed.Content); err != nil {
		slog.Warn("failed to process wiki-links", "path", path, "error", err)
	}

	// 10. Resolve dangling links that might point to this document
	s.resolveDanglingForDoc(ctx, input.VaultID, doc)

	// 11. Chunk and embed (if embedder available)
	if s.embedder != nil {
		if err := s.processChunks(ctx, docID, parsed, allLabels); err != nil {
			slog.Warn("failed to process chunks", "path", path, "error", err)
		}
	}

	// 12. Process explicit relates_to from frontmatter
	if created {
		s.processRelatesTo(ctx, docID, input.VaultID, parsed.Frontmatter)
	}

	return doc, nil
}

// Update re-runs the document pipeline on existing content.
func (s *Service) Update(ctx context.Context, vaultID, path, content string) (*models.Document, error) {
	return s.Create(ctx, models.DocumentInput{
		VaultID: vaultID,
		Path:    path,
		Content: content,
		Source:  models.SourceManual,
	})
}

// Delete removes a document and its associated data.
func (s *Service) Delete(ctx context.Context, vaultID, path string) error {
	doc, err := s.db.GetDocumentByPath(ctx, vaultID, path)
	if err != nil {
		return fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("document not found: %s", path)
	}

	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return fmt.Errorf("extract doc id: %w", err)
	}

	return s.db.DeleteDocument(ctx, docID)
}

// Move changes a document's path.
func (s *Service) Move(ctx context.Context, vaultID, oldPath, newPath string) (*models.Document, error) {
	doc, err := s.db.GetDocumentByPath(ctx, vaultID, oldPath)
	if err != nil {
		return nil, fmt.Errorf("get document: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("document not found: %s", oldPath)
	}

	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("extract doc id: %w", err)
	}

	return s.db.MoveDocument(ctx, docID, models.NormalizePath(newPath))
}

func (s *Service) processWikiLinks(ctx context.Context, docID, vaultID, content string) error {
	// Delete existing links from this doc
	if err := s.db.DeleteWikiLinks(ctx, docID); err != nil {
		return fmt.Errorf("delete old wiki-links: %w", err)
	}

	targets := parser.ExtractWikiLinks(content)
	if len(targets) == 0 {
		return nil
	}

	links := make([]db.WikiLinkInput, 0, len(targets))
	for _, target := range targets {
		var toDocID *string
		resolved, err := s.resolver.Resolve(ctx, vaultID, target)
		if err != nil {
			slog.Warn("failed to resolve wiki-link", "target", target, "error", err)
		} else if resolved != nil {
			id, err := models.RecordIDString(resolved.ID)
			if err != nil {
				slog.Warn("failed to extract resolved document ID for wiki-link", "target", target, "error", err)
			} else {
				toDocID = &id
			}
		}
		links = append(links, db.WikiLinkInput{
			RawTarget: target,
			ToDocID:   toDocID,
		})
	}

	return s.db.CreateWikiLinks(ctx, docID, vaultID, links)
}

func (s *Service) resolveDanglingForDoc(ctx context.Context, vaultID string, doc *models.Document) {
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		slog.Warn("failed to extract document ID for dangling link resolution", "path", doc.Path, "error", err)
		return
	}

	// Try to resolve by title
	if doc.Title != "" {
		if n, err := s.db.ResolveDanglingLinks(ctx, vaultID, doc.Title, docID); err != nil {
			slog.Warn("failed to resolve dangling links by title", "title", doc.Title, "error", err)
		} else if n > 0 {
			slog.Info("resolved dangling wiki-links", "title", doc.Title, "count", n)
		}
	}

	// Try to resolve by path
	if n, err := s.db.ResolveDanglingLinks(ctx, vaultID, doc.Path, docID); err != nil {
		slog.Warn("failed to resolve dangling links by path", "path", doc.Path, "error", err)
	} else if n > 0 {
		slog.Info("resolved dangling wiki-links", "path", doc.Path, "count", n)
	}
}

func (s *Service) processChunks(ctx context.Context, docID string, parsed *parser.MarkdownDoc, labels []string) error {
	// Delete existing chunks
	if err := s.db.DeleteChunks(ctx, docID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	chunkResults := parser.ChunkMarkdown(parsed, parser.DefaultChunkConfig())

	if len(chunkResults) == 0 {
		return nil
	}

	chunks := make([]models.ChunkInput, 0, len(chunkResults))
	for i, cr := range chunkResults {
		emb, err := s.embedder.Embed(ctx, cr.Content)
		if err != nil {
			slog.Warn("failed to embed chunk", "position", i, "error", err)
			continue
		}

		var headingPath *string
		if cr.HeadingPath != "" {
			headingPath = &cr.HeadingPath
		}

		chunks = append(chunks, models.ChunkInput{
			DocumentID:  docID,
			Content:     cr.Content,
			Position:    i,
			HeadingPath: headingPath,
			Labels:      labels,
			Embedding:   emb,
		})
	}

	return s.db.CreateChunks(ctx, chunks)
}

func (s *Service) processRelatesTo(ctx context.Context, docID, vaultID string, frontmatter map[string]any) {
	relatesTo, ok := frontmatter["relates_to"]
	if !ok {
		return
	}

	var targets []string
	switch v := relatesTo.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				targets = append(targets, s)
			}
		}
	case string:
		targets = []string{v}
	}

	for _, target := range targets {
		resolved, err := s.resolver.Resolve(ctx, vaultID, target)
		if err != nil {
			slog.Warn("failed to resolve relates_to target", "target", target, "error", err)
			continue
		}
		if resolved == nil {
			slog.Info("relates_to target not found", "target", target)
			continue
		}
		toDocID, err := models.RecordIDString(resolved.ID)
		if err != nil {
			slog.Warn("failed to extract resolved document ID for relates_to", "target", target, "error", err)
			continue
		}
		if _, err := s.db.CreateRelation(ctx, models.DocRelationInput{
			FromDocID: docID,
			ToDocID:   toDocID,
			RelType:   string(models.RelRelatesTo),
			Source:    string(models.RelSourceFrontmatter),
		}); err != nil {
			slog.Warn("failed to create relates_to relation", "target", target, "error", err)
		}
	}
}

// filenameTitle extracts a title from a file path.
func filenameTitle(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return cases.Title(language.English).String(name)
}

// extractMetadata extracts non-standard frontmatter keys as metadata.
func extractMetadata(fm map[string]any) map[string]any {
	standard := map[string]bool{
		"title": true, "name": true, "labels": true, "tags": true,
		"type": true, "relates_to": true, "description": true,
	}

	metadata := make(map[string]any)
	for k, v := range fm {
		if !standard[k] {
			metadata[k] = v
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}
