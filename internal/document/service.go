package document

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/event"
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
	db                 *db.Client
	embedder           atomic.Pointer[llm.Embedder] // optional — nil disables embedding
	resolver           *LinkResolver
	chunkConfig        parser.ChunkConfig
	versionConfig      VersionConfig
	bus                *event.Bus // optional — nil disables change events
	embedMaxInputChars int        // hard limit for embedding API input (0 = no limit)
}

// SetEmbedder atomically replaces the embedder (used by SIGHUP reload).
func (s *Service) SetEmbedder(e *llm.Embedder) {
	s.embedder.Store(e)
}

// getEmbedder returns the current embedder via an atomic load.
func (s *Service) getEmbedder() *llm.Embedder {
	return s.embedder.Load()
}

// NewService creates a new document service.
// embedMaxInputChars is the hard character limit for embedding API input (0 = no limit).
func NewService(db *db.Client, embedder *llm.Embedder, chunkConfig parser.ChunkConfig, versionConfig VersionConfig, bus *event.Bus, embedMaxInputChars int) *Service {
	if versionConfig.RetentionCount < 1 {
		slog.Warn("version retention count too low, clamping to 1", "configured", versionConfig.RetentionCount)
		versionConfig.RetentionCount = 1
	}
	if versionConfig.CoalesceMinutes < 0 {
		slog.Warn("version coalesce minutes negative, clamping to 0", "configured", versionConfig.CoalesceMinutes)
		versionConfig.CoalesceMinutes = 0
	}
	s := &Service{
		db:                 db,
		resolver:           NewLinkResolver(db),
		chunkConfig:        chunkConfig,
		versionConfig:      versionConfig,
		bus:                bus,
		embedMaxInputChars: embedMaxInputChars,
	}
	s.embedder.Store(embedder)
	return s
}

func (s *Service) publishDocEvent(eventType string, vaultID string, doc *models.Document) {
	if s.bus == nil {
		return
	}
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		slog.Warn("failed to extract doc ID for event", "error", err)
		return
	}
	var contentHash string
	if doc.ContentHash != nil {
		contentHash = *doc.ContentHash
	}
	s.bus.Publish(event.ChangeEvent{
		Type:    eventType,
		VaultID: vaultID,
		Payload: event.DocumentPayload{
			DocID:       docID,
			Path:        doc.Path,
			ContentHash: contentHash,
		},
	})
}

func (s *Service) publishDocDeleteEvent(vaultID, docID, path, contentHash string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(event.ChangeEvent{
		Type:    "document.deleted",
		VaultID: vaultID,
		Payload: event.DocumentPayload{
			DocID:       docID,
			Path:        path,
			ContentHash: contentHash,
		},
	})
}

func (s *Service) publishDocMoveEvent(vaultID string, doc *models.Document, oldPath string) {
	if s.bus == nil {
		return
	}
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		slog.Warn("failed to extract doc ID for move event", "error", err)
		return
	}
	var contentHash string
	if doc.ContentHash != nil {
		contentHash = *doc.ContentHash
	}
	s.bus.Publish(event.ChangeEvent{
		Type:    "document.moved",
		VaultID: vaultID,
		Payload: event.DocumentPayload{
			DocID:       docID,
			Path:        doc.Path,
			OldPath:     oldPath,
			ContentHash: contentHash,
		},
	})
}

// Create stores a document with fast-path parsing and defers heavy processing
// (chunks, wiki-links, relations) to the async ProcessingWorker.
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

	// Create version snapshot of old content (if this was an update with changed content)
	if !created && previousDoc != nil {
		docID, idErr := models.RecordIDString(doc.ID)
		if idErr != nil {
			slog.Warn("failed to extract doc ID for versioning", "error", idErr)
		} else {
			s.maybeCreateVersion(ctx, docID, input.VaultID, previousDoc, contentHash)
		}
	}

	// Publish change event (document is stored but not yet processed)
	if created {
		s.publishDocEvent("document.created", input.VaultID, doc)
	} else {
		s.publishDocEvent("document.updated", input.VaultID, doc)
	}

	return doc, nil
}

// ProcessDocument runs the deferred heavy processing pipeline for a document:
// chunks, wiki-links, dangling link resolution, and relations.
// Called by the ProcessingWorker for documents with processed = false.
func (s *Service) ProcessDocument(ctx context.Context, doc *models.Document) error {
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return fmt.Errorf("extract doc id: %w", err)
	}

	vaultID, err := models.RecordIDString(doc.Vault)
	if err != nil {
		return fmt.Errorf("extract vault id: %w", err)
	}

	parsed, err := parser.ParseMarkdown(doc.Content)
	if err != nil {
		return fmt.Errorf("parse markdown: %w", err)
	}

	// 1. Sync chunks (with smart diffing — only re-embed changed chunks)
	if err := s.syncChunks(ctx, docID, parsed, doc.Labels); err != nil {
		return fmt.Errorf("sync chunks for %s: %w", doc.Path, err)
	}

	// 2. Extract and store wiki-links
	if err := s.processWikiLinks(ctx, docID, vaultID, parsed.Content); err != nil {
		return fmt.Errorf("process wiki-links for %s: %w", doc.Path, err)
	}

	// 3. Resolve dangling links that might point to this document
	if err := s.resolveDanglingForDoc(ctx, vaultID, doc); err != nil {
		return fmt.Errorf("resolve dangling links for %s: %w", doc.Path, err)
	}

	// 4. Process explicit relates_to from frontmatter
	if err := s.processRelatesTo(ctx, docID, vaultID, parsed.Frontmatter); err != nil {
		return fmt.Errorf("process relates_to for %s: %w", doc.Path, err)
	}

	// 5. Mark as processed
	if err := s.db.MarkDocumentProcessed(ctx, docID); err != nil {
		return fmt.Errorf("mark processed: %w", err)
	}

	// 6. Publish processed event
	s.publishDocEvent("document.processed", vaultID, doc)

	return nil
}

// ProcessAllPending processes all unprocessed documents synchronously.
// Intended for tests and CLI commands that need immediate processing.
func (s *Service) ProcessAllPending(ctx context.Context) error {
	for {
		docs, err := s.db.ListUnprocessedDocuments(ctx, 100)
		if err != nil {
			return fmt.Errorf("list unprocessed: %w", err)
		}
		if len(docs) == 0 {
			return nil
		}
		for _, doc := range docs {
			if err := s.ProcessDocument(ctx, &doc); err != nil {
				return fmt.Errorf("process %s: %w", doc.Path, err)
			}
		}
	}
}

// EmbedPendingChunks claims chunks that are due for embedding and embeds them.
// Uses contextual retrieval: prepends document title and section path to chunk
// content before embedding, improving semantic precision without altering stored content.
// Returns the number of chunks successfully embedded.
func (s *Service) EmbedPendingChunks(ctx context.Context, limit int) (int, error) {
	embedder := s.getEmbedder()
	if embedder == nil {
		return 0, nil
	}

	chunks, err := s.db.ClaimChunksForEmbedding(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("claim chunks for embedding: %w", err)
	}

	// Batch-fetch document titles for contextual embedding
	docTitles := s.fetchDocTitles(ctx, chunks)

	embedded := 0
	for _, chunk := range chunks {
		chunkID, err := models.RecordIDString(chunk.ID)
		if err != nil {
			slog.Warn("failed to extract chunk ID for embedding", "error", err)
			continue
		}

		docID, err := models.RecordIDString(chunk.Document)
		if err != nil {
			slog.Warn("failed to extract document ID for contextual embedding", "chunk_id", chunkID, "error", err)
			continue
		}
		embeddingText := buildEmbeddingContext(chunk, docTitles[docID], s.embedMaxInputChars)

		emb, err := embedder.Embed(ctx, embeddingText)
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

// fetchDocTitles collects unique document IDs from a batch of chunks and
// batch-fetches their titles. Returns a map of docID → title.
func (s *Service) fetchDocTitles(ctx context.Context, chunks []models.Chunk) map[string]string {
	titles := make(map[string]string)

	// Collect unique doc IDs
	docIDSet := make(map[string]struct{})
	for _, chunk := range chunks {
		docID, err := models.RecordIDString(chunk.Document)
		if err != nil {
			slog.Warn("failed to extract document ID for title lookup", "error", err)
			continue
		}
		docIDSet[docID] = struct{}{}
	}
	docIDs := make([]string, 0, len(docIDSet))
	for id := range docIDSet {
		docIDs = append(docIDs, id)
	}

	if len(docIDs) == 0 {
		return titles
	}

	docs, err := s.db.GetDocumentsByIDs(ctx, docIDs)
	if err != nil {
		slog.Warn("failed to fetch document titles for contextual embedding", "error", err)
		return titles
	}

	for _, doc := range docs {
		docID, err := models.RecordIDString(doc.ID)
		if err != nil {
			slog.Warn("failed to extract document ID from fetched doc", "error", err)
			continue
		}
		titles[docID] = doc.Title
	}

	return titles
}

// buildEmbeddingContext prepends document and section context to chunk content
// for better embedding quality (contextual retrieval technique).
// The context prefix is only used at embedding time, not stored in the chunk.
// If maxChars > 0 and the assembled string exceeds maxChars, the content is
// truncated at a word boundary (the prefix is preserved).
func buildEmbeddingContext(chunk models.Chunk, docTitle string, maxChars int) string {
	var b strings.Builder
	if docTitle != "" {
		fmt.Fprintf(&b, "Document: %s\n", docTitle)
	}
	if chunk.HeadingPath != nil && *chunk.HeadingPath != "" {
		section := stripMarkdownHeadingPrefixes(*chunk.HeadingPath)
		fmt.Fprintf(&b, "Section: %s\n", section)
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	prefixLen := b.Len()

	b.WriteString(chunk.Content)
	result := b.String()

	if maxChars > 0 && len(result) > maxChars {
		// Truncate content, keeping prefix intact.
		contentBudget := maxChars - prefixLen
		if contentBudget < 0 {
			contentBudget = 0
		}
		result = result[:prefixLen] + truncateAtWordBoundary(chunk.Content, contentBudget)
	}
	return result
}

// truncateAtWordBoundary truncates s to at most maxLen characters,
// cutting at the last space boundary to avoid splitting words.
func truncateAtWordBoundary(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 0 {
		return ""
	}
	truncated := s[:maxLen]
	if idx := strings.LastIndexByte(truncated, ' '); idx > maxLen/2 {
		truncated = truncated[:idx]
	}
	return truncated
}

// stripMarkdownHeadingPrefixes removes markdown heading markers from a heading path.
// e.g. "## Setup > ### Install" → "Setup > Install"
func stripMarkdownHeadingPrefixes(path string) string {
	parts := strings.Split(path, " > ")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(strings.TrimLeft(part, "#"))
	}
	return strings.Join(parts, " > ")
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

	var contentHash string
	if doc.ContentHash != nil {
		contentHash = *doc.ContentHash
	}

	// Synchronous cleanup — async cascade events are a safety net, not primary.
	if err := s.db.DeleteWikiLinks(ctx, docID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if err := s.db.DeleteChunks(ctx, docID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	if err := s.db.DeleteDocument(ctx, docID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	s.publishDocDeleteEvent(vaultID, docID, path, contentHash)

	return nil
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
		return 0, fmt.Errorf("delete by prefix: %w", err)
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
		return 0, fmt.Errorf("move by prefix: %w", err)
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

	s.publishDocMoveEvent(vaultID, doc, oldPath)

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

func (s *Service) resolveDanglingForDoc(ctx context.Context, vaultID string, doc *models.Document) error {
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return fmt.Errorf("extract document id: %w", err)
	}

	// Try to resolve by title
	if doc.Title != "" {
		n, err := s.db.ResolveDanglingLinks(ctx, vaultID, doc.Title, docID)
		if err != nil {
			return fmt.Errorf("resolve by title %q: %w", doc.Title, err)
		}
		if n > 0 {
			slog.Info("resolved dangling wiki-links", "title", doc.Title, "count", n)
		}
	}

	// Try to resolve by path
	n, err := s.db.ResolveDanglingLinks(ctx, vaultID, doc.Path, docID)
	if err != nil {
		return fmt.Errorf("resolve by path %q: %w", doc.Path, err)
	}
	if n > 0 {
		slog.Info("resolved dangling wiki-links", "path", doc.Path, "count", n)
	}

	return nil
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
			if s.getEmbedder() != nil {
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

func (s *Service) processRelatesTo(ctx context.Context, docID, vaultID string, frontmatter map[string]any) error {
	relatesTo, ok := frontmatter["relates_to"]
	if !ok {
		return nil
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
			return fmt.Errorf("resolve relates_to target %q: %w", target, err)
		}
		if resolved == nil {
			slog.Info("relates_to target not found", "target", target)
			continue
		}
		toDocID, err := models.RecordIDString(resolved.ID)
		if err != nil {
			return fmt.Errorf("extract resolved document id for %q: %w", target, err)
		}
		if _, err := s.db.CreateRelation(ctx, models.DocRelationInput{
			FromDocID: docID,
			ToDocID:   toDocID,
			RelType:   string(models.RelRelatesTo),
			Source:    string(models.RelSourceFrontmatter),
		}); err != nil {
			return fmt.Errorf("create relates_to relation for %q: %w", target, err)
		}
	}

	return nil
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
