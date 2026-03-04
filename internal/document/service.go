package document

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"

	"github.com/raphi011/knowhow/internal/llm"
	"github.com/raphi011/knowhow/internal/parser"
)

// VersionConfig holds versioning settings.
type VersionConfig struct {
	CoalesceMinutes int // minutes between version snapshots
	RetentionCount  int // max versions per document
}

// Service manages document lifecycle: parse → extract → store → link → embed.
type Service struct {
	db            *db.Client
	embedder      *llm.Embedder // optional — nil disables embedding
	resolver      *LinkResolver
	chunkConfig   parser.ChunkConfig
	versionConfig VersionConfig
}

// NewService creates a new document service.
func NewService(db *db.Client, embedder *llm.Embedder, chunkConfig parser.ChunkConfig, versionConfig VersionConfig) *Service {
	return &Service{
		db:            db,
		embedder:      embedder,
		resolver:      NewLinkResolver(db),
		chunkConfig:   chunkConfig,
		versionConfig: versionConfig,
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

	// Auto-create parent folders before document upsert
	if err := s.db.EnsureFolders(ctx, input.VaultID, path); err != nil {
		return nil, fmt.Errorf("ensure parent folders: %w", err)
	}

	doc, created, previousDoc, err := s.db.UpsertDocument(ctx, dbInput)
	if err != nil {
		return nil, fmt.Errorf("upsert document: %w", err)
	}

	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("extract doc id: %w", err)
	}

	// 8.5 Create version snapshot of old content (if this was an update)
	if !created && previousDoc != nil {
		s.maybeCreateVersion(ctx, docID, input.VaultID, previousDoc, contentHash)
	}

	// 9. Sync chunks (with smart diffing — only re-embed changed chunks)
	if err := s.syncChunks(ctx, docID, parsed, allLabels); err != nil {
		slog.Warn("failed to sync chunks", "path", path, "error", err)
	}

	// 10. Extract and store wiki-links
	if err := s.processWikiLinks(ctx, docID, input.VaultID, parsed.Content); err != nil {
		slog.Warn("failed to process wiki-links", "path", path, "error", err)
	}

	// 11. Resolve dangling links that might point to this document
	s.resolveDanglingForDoc(ctx, input.VaultID, doc)

	// 12. Process explicit relates_to from frontmatter
	if created {
		s.processRelatesTo(ctx, docID, input.VaultID, parsed.Frontmatter)
	}

	return doc, nil
}

// EmbedPendingChunks claims chunks that are due for embedding and embeds them.
// Returns the number of chunks successfully embedded.
func (s *Service) EmbedPendingChunks(ctx context.Context, limit int) (int, error) {
	if s.embedder == nil {
		return 0, nil
	}

	chunks, err := s.db.ClaimChunksForEmbedding(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("claim chunks for embedding: %w", err)
	}

	embedded := 0
	for _, chunk := range chunks {
		chunkID, err := models.RecordIDString(chunk.ID)
		if err != nil {
			slog.Warn("failed to extract chunk ID for embedding", "error", err)
			continue
		}

		emb, err := s.embedder.Embed(ctx, chunk.Content)
		if err != nil {
			slog.Warn("failed to embed chunk", "chunk_id", chunkID, "error", err)
			s.rescheduleChunk(chunkID)
			continue
		}

		if err := s.db.UpdateChunkEmbedding(ctx, chunkID, emb); err != nil {
			slog.Warn("failed to store chunk embedding", "chunk_id", chunkID, "error", err)
			s.rescheduleChunk(chunkID)
			continue
		}

		embedded++
	}

	return embedded, nil
}

// rescheduleChunk re-schedules a chunk for embedding after a failure, using a
// background context so it succeeds even if the parent context is cancelled.
// Uses a 30s backoff to avoid tight retry storms during outages.
func (s *Service) rescheduleChunk(chunkID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.db.RescheduleChunkEmbedding(ctx, chunkID); err != nil {
		slog.Error("failed to reschedule chunk embedding — chunk will not be retried",
			"chunk_id", chunkID, "error", err)
	}
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
	path = models.NormalizePath(path)
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

// DeleteByPrefix removes all documents whose path starts with the given prefix.
// Returns the number of deleted documents. The prefix is normalized and ensured
// to end with "/" to avoid matching paths like "/guides-extra" when deleting "/guides".
//
// Cleanup of chunks, wiki-links, and relations relies on SurrealDB's async cascade
// events, so associated data is eventually consistent after a bulk delete.
func (s *Service) DeleteByPrefix(ctx context.Context, vaultID, pathPrefix string) (int, error) {
	prefix := models.NormalizePath(pathPrefix)
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if prefix == "/" {
		return 0, fmt.Errorf("delete by prefix: refusing to delete root prefix")
	}
	count, err := s.db.DeleteDocumentsByPrefix(ctx, vaultID, prefix)
	if err != nil {
		return 0, err
	}

	// Clean up folder records under the same prefix
	if _, err := s.db.DeleteFoldersByPrefix(ctx, vaultID, prefix); err != nil {
		return count, fmt.Errorf("delete folder records: %w", err)
	}

	return count, nil
}

// MoveByPrefix renames all documents whose path starts with oldPrefix,
// replacing oldPrefix with newPrefix. Returns the number of moved documents.
// Both prefixes are normalized and ensured to end with "/" to avoid partial matches.
//
// Does not update wiki-links or relations referencing the old paths.
func (s *Service) MoveByPrefix(ctx context.Context, vaultID, oldPrefix, newPrefix string) (int, error) {
	oldNorm := models.NormalizePath(oldPrefix)
	if !strings.HasSuffix(oldNorm, "/") {
		oldNorm += "/"
	}
	newNorm := models.NormalizePath(newPrefix)
	if !strings.HasSuffix(newNorm, "/") {
		newNorm += "/"
	}
	if oldNorm == "/" {
		return 0, fmt.Errorf("move by prefix: refusing to move root prefix")
	}
	if oldNorm == newNorm {
		return 0, nil
	}
	count, err := s.db.MoveDocumentsByPrefix(ctx, vaultID, oldNorm, newNorm)
	if err != nil {
		return 0, err
	}

	// Move folder records to match
	if _, err := s.db.MoveFoldersByPrefix(ctx, vaultID, oldNorm, newNorm); err != nil {
		return count, fmt.Errorf("move folder records: %w", err)
	}
	// Ensure destination ancestor folders exist
	if err := s.db.EnsureFolderPath(ctx, vaultID, strings.TrimSuffix(newNorm, "/")); err != nil {
		return count, fmt.Errorf("ensure destination folders: %w", err)
	}

	return count, nil
}

// Move changes a document's path.
func (s *Service) Move(ctx context.Context, vaultID, oldPath, newPath string) (*models.Document, error) {
	oldPath = models.NormalizePath(oldPath)
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

	normalizedNew := models.NormalizePath(newPath)
	doc, err = s.db.MoveDocument(ctx, docID, normalizedNew)
	if err != nil {
		return nil, fmt.Errorf("move document: %w", err)
	}

	// Ensure destination folders exist
	if err := s.db.EnsureFolders(ctx, vaultID, normalizedNew); err != nil {
		return nil, fmt.Errorf("ensure destination folders: %w", err)
	}

	return doc, nil
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

// syncChunks performs smart chunk diffing: compares new chunks against existing ones
// by content, preserving embeddings for unchanged chunks and scheduling embedding
// only for new/changed chunks.
func (s *Service) syncChunks(ctx context.Context, docID string, parsed *parser.MarkdownDoc, labels []string) error {
	newChunkResults := parser.ChunkMarkdown(parsed, s.chunkConfig)

	oldChunks, err := s.db.GetChunks(ctx, docID)
	if err != nil {
		return fmt.Errorf("get existing chunks: %w", err)
	}

	// Build lookup: content → old chunk (for content-based matching).
	// Also collect all resolved IDs so we don't need to call RecordIDString twice.
	type oldChunkEntry struct {
		chunk models.Chunk
		id    string
	}
	oldByContent := make(map[string]*oldChunkEntry, len(oldChunks))
	allOldIDs := make([]string, 0, len(oldChunks))
	for _, c := range oldChunks {
		id, err := models.RecordIDString(c.ID)
		if err != nil {
			slog.Warn("failed to extract chunk ID during sync", "error", err)
			continue
		}
		allOldIDs = append(allOldIDs, id)
		// Only store first occurrence per content (handles duplicates)
		if _, exists := oldByContent[c.Content]; !exists {
			oldByContent[c.Content] = &oldChunkEntry{chunk: c, id: id}
		}
	}

	matchedOldIDs := make(map[string]bool)
	var toCreate []models.ChunkInput

	for i, newChunk := range newChunkResults {
		if entry, ok := oldByContent[newChunk.Content]; ok && !matchedOldIDs[entry.id] {
			// Content unchanged — keep existing chunk (preserve embedding)
			matchedOldIDs[entry.id] = true
			// Update position if it changed
			if entry.chunk.Position != i {
				if err := s.db.UpdateChunkPosition(ctx, entry.id, i); err != nil {
					slog.Warn("failed to update chunk position", "chunk_id", entry.id, "error", err)
				}
			}
		} else {
			// New or changed chunk — create with embed_at
			var headingPath *string
			if newChunk.HeadingPath != "" {
				headingPath = &newChunk.HeadingPath
			}

			input := models.ChunkInput{
				DocumentID:  docID,
				Content:     newChunk.Content,
				Position:    i,
				HeadingPath: headingPath,
				Labels:      labels,
				Embedding:   nil, // nil until worker fills it
			}

			// Only schedule embedding if embedder is configured
			if s.embedder != nil {
				now := time.Now().UTC()
				input.EmbedAt = &now
			}

			toCreate = append(toCreate, input)
		}
	}

	// Delete old chunks that were not matched (removed content)
	for _, id := range allOldIDs {
		if !matchedOldIDs[id] {
			if err := s.db.DeleteChunkByID(ctx, id); err != nil {
				slog.Warn("failed to delete removed chunk", "chunk_id", id, "error", err)
			}
		}
	}

	// Create new chunks
	if len(toCreate) > 0 {
		if err := s.db.CreateChunks(ctx, toCreate); err != nil {
			return fmt.Errorf("create new chunks: %w", err)
		}
	}

	return nil
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
