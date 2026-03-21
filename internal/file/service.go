package file

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/pipeline"
	"github.com/raphi011/know/internal/stt"
)

// vaultEmbedEntry caches vault embedding enabled status for a short duration.
type vaultEmbedEntry struct {
	enabled   bool
	expiresAt time.Time
}

// VersionConfig holds versioning settings.
type VersionConfig struct {
	CoalesceMinutes int // minutes between version snapshots
	RetentionCount  int // max versions per file
}

// Service manages file lifecycle: parse → extract → store → link → embed.
type Service struct {
	db                  *db.Client
	blobStore           blob.Store
	embedder            atomic.Pointer[llm.Embedder]           // optional — nil disables embedding
	multimodalEmbedder  atomic.Pointer[llm.MultimodalEmbedder] // optional — nil disables multimodal embedding
	textExtractor       atomic.Pointer[llm.TextExtractor]      // optional — nil disables LLM text extraction
	transcriber         atomic.Pointer[stt.Transcriber]        // optional — nil disables transcription
	model               atomic.Pointer[llm.Model]              // optional — nil disables LLM summarization
	resolver            *LinkResolver
	chunkConfig         parser.ChunkConfig
	versionConfig       VersionConfig
	bus                 *event.Bus // optional — nil disables change events
	embedMaxInputChars  int        // hard limit for embedding API input (0 = no limit)
	audioSegmentSeconds int        // max audio segment duration for chunking (default 60)
	pdfRenderDPI        int        // DPI for PDF page rendering (default 300)

	// vaultEmbedCache caches vault embedding toggle to avoid N+1 DB lookups on bulk operations.
	vaultEmbedMu    sync.Mutex
	vaultEmbedCache map[string]vaultEmbedEntry
}

// SetEmbedder atomically replaces the embedder (used by SIGHUP reload).
func (s *Service) SetEmbedder(e *llm.Embedder) {
	s.embedder.Store(e)
}

// SetChunkConfig updates the chunk config and embed limit (used by SIGHUP reload).
func (s *Service) SetChunkConfig(cc parser.ChunkConfig, embedMaxInputChars int) {
	s.chunkConfig = cc
	s.embedMaxInputChars = embedMaxInputChars
}

// getEmbedder returns the current embedder via an atomic load.
func (s *Service) getEmbedder() *llm.Embedder {
	return s.embedder.Load()
}

// SetTranscriber atomically replaces the transcriber (used by SIGHUP reload).
func (s *Service) SetTranscriber(t *stt.Transcriber) {
	s.transcriber.Store(t)
}

func (s *Service) getTranscriber() *stt.Transcriber {
	return s.transcriber.Load()
}

// SetModel atomically replaces the LLM model (used by SIGHUP reload).
func (s *Service) SetModel(m *llm.Model) {
	s.model.Store(m)
}

func (s *Service) getModel() *llm.Model {
	return s.model.Load()
}

// SetMultimodalEmbedder atomically replaces the multimodal embedder (used by SIGHUP reload).
func (s *Service) SetMultimodalEmbedder(e *llm.MultimodalEmbedder) {
	s.multimodalEmbedder.Store(e)
}

func (s *Service) getMultimodalEmbedder() *llm.MultimodalEmbedder {
	return s.multimodalEmbedder.Load()
}

// SetTextExtractor atomically replaces the text extractor (used by SIGHUP reload).
func (s *Service) SetTextExtractor(e *llm.TextExtractor) {
	s.textExtractor.Store(e)
}

func (s *Service) getTextExtractor() *llm.TextExtractor {
	return s.textExtractor.Load()
}

// SetAudioSegmentSeconds updates the audio segment duration from config.
func (s *Service) SetAudioSegmentSeconds(seconds int) {
	if seconds <= 0 {
		seconds = 60
	}
	s.audioSegmentSeconds = seconds
}

// SetPDFRenderDPI updates the DPI used for PDF page rendering (used by SIGHUP reload).
func (s *Service) SetPDFRenderDPI(dpi int) {
	if dpi <= 0 {
		dpi = 300
	}
	s.pdfRenderDPI = dpi
}

// vaultEmbeddingEnabled checks the vault-level embedding toggle with a short cache
// to avoid N+1 DB lookups during bulk operations. Fails open on error.
func (s *Service) vaultEmbeddingEnabled(ctx context.Context, vaultID string) bool {
	const cacheTTL = 1 * time.Minute

	s.vaultEmbedMu.Lock()
	if s.vaultEmbedCache == nil {
		s.vaultEmbedCache = make(map[string]vaultEmbedEntry)
	}
	if entry, ok := s.vaultEmbedCache[vaultID]; ok && time.Now().Before(entry.expiresAt) {
		s.vaultEmbedMu.Unlock()
		return entry.enabled
	}
	s.vaultEmbedMu.Unlock()

	vault, err := s.db.GetVault(ctx, vaultID)
	if err != nil {
		logutil.FromCtx(ctx).Warn("failed to load vault for embed check, defaulting to enabled", "vault_id", vaultID, "error", err)
		return true // fail open but don't cache — next call retries the DB lookup
	}

	enabled := true
	if vault != nil {
		enabled = vault.Defaults().IsEmbeddingEnabled()
	}

	s.vaultEmbedMu.Lock()
	s.vaultEmbedCache[vaultID] = vaultEmbedEntry{enabled: enabled, expiresAt: time.Now().Add(cacheTTL)}
	s.vaultEmbedMu.Unlock()

	return enabled
}

// shouldEmbed returns true if the file at filePath should have embeddings generated.
// It checks vault-level embedding toggle first (cached), then whether any ancestor
// folder has no_embed = true. Fails open (returns true) on error.
func (s *Service) shouldEmbed(ctx context.Context, vaultID, filePath string) bool {
	if !s.vaultEmbeddingEnabled(ctx, vaultID) {
		return false
	}

	noEmbed, err := s.db.IsPathNoEmbed(ctx, vaultID, filePath)
	if err != nil {
		logutil.FromCtx(ctx).Warn("failed to check no_embed, proceeding with embedding", "vault_id", vaultID, "path", filePath, "error", err)
		return true
	}
	return !noEmbed
}

// BlobStore returns the blob store used by this service.
func (s *Service) BlobStore() blob.Store { return s.blobStore }

// storeTranscript stores text content in the blob store and updates the
// file's hash and size in the DB.
func (s *Service) storeTranscript(ctx context.Context, fileID, text string) error {
	hash := models.ContentHash(text)
	if err := s.blobStore.Put(ctx, hash, strings.NewReader(text), int64(len(text))); err != nil {
		return fmt.Errorf("store blob: %w", err)
	}
	if err := s.db.UpdateFileHashAndSize(ctx, fileID, hash, len(text)); err != nil {
		return fmt.Errorf("update hash and size: %w", err)
	}
	return nil
}

// ReadContent loads file content from the blob store by content hash.
// Returns "" for empty hashes (folders, files with no content).
func (s *Service) ReadContent(ctx context.Context, contentHash string) (string, error) {
	if contentHash == "" {
		return "", nil
	}
	rc, err := s.blobStore.Get(ctx, contentHash)
	if err != nil {
		return "", fmt.Errorf("read content blob %s: %w", contentHash, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("read content blob %s: %w", contentHash, err)
	}
	return string(data), nil
}

// ReadFileContent loads content for a file from the blob store.
// Convenience wrapper that handles nil ContentHash.
func (s *Service) ReadFileContent(ctx context.Context, f *models.File) (string, error) {
	if f.Hash == nil {
		return "", nil
	}
	return s.ReadContent(ctx, *f.Hash)
}

// NewService creates a new file service.
// embedMaxInputChars is the hard character limit for embedding API input (0 = no limit).
func NewService(db *db.Client, blobStore blob.Store, embedder *llm.Embedder, chunkConfig parser.ChunkConfig, versionConfig VersionConfig, bus *event.Bus, embedMaxInputChars int) *Service {
	if versionConfig.RetentionCount < 1 {
		slog.Warn("version retention count too low, clamping to 1", "configured", versionConfig.RetentionCount)
		versionConfig.RetentionCount = 1
	}
	if versionConfig.CoalesceMinutes < 0 {
		slog.Warn("version coalesce minutes negative, clamping to 0", "configured", versionConfig.CoalesceMinutes)
		versionConfig.CoalesceMinutes = 0
	}
	s := &Service{
		db:                  db,
		blobStore:           blobStore,
		resolver:            NewLinkResolver(db),
		chunkConfig:         chunkConfig,
		versionConfig:       versionConfig,
		bus:                 bus,
		embedMaxInputChars:  embedMaxInputChars,
		audioSegmentSeconds: 60,
	}
	s.embedder.Store(embedder)
	return s
}

func (s *Service) publishFileEvent(eventType string, vaultID string, doc *models.File) {
	if s.bus == nil {
		return
	}
	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		slog.Warn("failed to extract file ID for event", "event", eventType, "error", err)
		return
	}
	var contentHash string
	if doc.Hash != nil {
		contentHash = *doc.Hash
	}
	s.bus.Publish(event.ChangeEvent{
		Type:    eventType,
		VaultID: vaultID,
		Payload: event.DocumentPayload{
			DocID: fileID,
			Path:  doc.Path,
			Hash:  contentHash,
		},
	})
}

func (s *Service) publishFileDeleteEvent(vaultID, fileID, path, contentHash string) {
	if s.bus == nil {
		return
	}
	s.bus.Publish(event.ChangeEvent{
		Type:    "file.deleted",
		VaultID: vaultID,
		Payload: event.DocumentPayload{
			DocID: fileID,
			Path:  path,
			Hash:  contentHash,
		},
	})
}

func (s *Service) publishFileMoveEvent(vaultID string, doc *models.File, oldPath string) {
	if s.bus == nil {
		return
	}
	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		slog.Warn("failed to extract file ID for move event", "error", err)
		return
	}
	var contentHash string
	if doc.Hash != nil {
		contentHash = *doc.Hash
	}
	s.bus.Publish(event.ChangeEvent{
		Type:    "file.moved",
		VaultID: vaultID,
		Payload: event.DocumentPayload{
			DocID:   fileID,
			Path:    doc.Path,
			OldPath: oldPath,
			Hash:    contentHash,
		},
	})
}

// Create stores a file with fast-path parsing and defers heavy processing
// (chunks, wiki-links, relations) to the async ProcessingWorker.
func (s *Service) Create(ctx context.Context, input models.FileInput) (*models.File, error) {
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("validate input: %w", err)
	}

	// Auto-detect MimeType from file extension when not provided.
	if input.MimeType == "" {
		input.MimeType = models.DetectMimeType(input.Path)
	}

	isBinary := len(input.Data) > 0

	var (
		title    string
		labels   []string
		docType  *string
		metadata map[string]any
	)

	if !isBinary {
		// Text file: parse frontmatter, extract labels/title/metadata
		parsed := parser.ParseMarkdown(input.Content)

		title = parsed.Title
		if title == "" {
			title = filenameTitle(input.Path)
		}

		fmLabels := parsed.GetFrontmatterStringSlice("labels")
		if fmLabels == nil {
			fmLabels = parsed.GetFrontmatterStringSlice("tags")
		}
		labels = MergeLabels(parsed.InlineLabels, append(fmLabels, input.Labels...))

		docType = input.DocType
		if docType == nil {
			if dt := parsed.GetFrontmatterString("type"); dt != "" {
				docType = &dt
			}
		}

		metadata = input.Metadata
		if metadata == nil {
			metadata = extractMetadata(parsed.Frontmatter)
		}
	} else {
		// Binary file: skip markdown parsing, use input values directly
		title = filenameTitle(input.Path)
		labels = input.Labels
		docType = input.DocType
		metadata = input.Metadata
	}

	// Compute content_hash and store content in blob
	var contentHash string
	var contentLength int

	if len(input.Data) > 0 {
		// Binary file: compute hash from Data and store in blob
		h := sha256.Sum256(input.Data)
		contentHash = hex.EncodeToString(h[:])
		contentLength = len(input.Data)
		if err := s.blobStore.Put(ctx, contentHash, bytes.NewReader(input.Data), int64(len(input.Data))); err != nil {
			return nil, fmt.Errorf("store blob: %w", err)
		}
	} else if input.Content != "" {
		// Text file: compute hash from Content and store in blob.
		contentHash = models.ContentHash(input.Content)
		contentLength = len(input.Content)
		if err := s.blobStore.Put(ctx, contentHash, strings.NewReader(input.Content), int64(len(input.Content))); err != nil {
			return nil, fmt.Errorf("store content blob: %w", err)
		}
	}

	// Normalize path
	path := models.NormalizePath(input.Path)

	// Store metadata in DB (content is in blob store)
	var hashPtr *string
	if contentHash != "" {
		hashPtr = &contentHash
	}
	dbInput := models.FileInput{
		VaultID:  input.VaultID,
		Path:     path,
		Title:    title,
		Hash:     hashPtr,
		Size:     contentLength,
		Labels:   labels,
		DocType:  docType,
		Metadata: metadata,
		MimeType: input.MimeType,
		IsFolder: input.IsFolder,
		Data:     input.Data, // carried for fileSize() — not stored in DB
	}

	return s.postUpsert(ctx, input.VaultID, dbInput, contentHash)
}

// CreateBinaryFromHash creates or updates DB metadata for a binary file whose blob
// is already stored in the blob store. This is used by the streaming import path where
// the blob was written via PutVerified before this method is called.
// Unlike Create(), this method does not read or buffer the file data.
func (s *Service) CreateBinaryFromHash(ctx context.Context, input models.FileInput) (*models.File, error) {
	if input.Hash == nil {
		return nil, fmt.Errorf("content hash required")
	}
	if input.MimeType == "" {
		input.MimeType = models.DetectMimeType(input.Path)
	}

	title := filenameTitle(input.Path)
	path := models.NormalizePath(input.Path)

	dbInput := models.FileInput{
		VaultID:  input.VaultID,
		Path:     path,
		Title:    title,
		Hash:     input.Hash,
		Labels:   input.Labels,
		DocType:  input.DocType,
		Metadata: input.Metadata,
		MimeType: input.MimeType,
		Size:     input.Size,
	}

	return s.postUpsert(ctx, input.VaultID, dbInput, *input.Hash)
}

// postUpsert handles the shared lifecycle after building a FileInput:
// ensure folders → upsert → version snapshot → publish event → enqueue job.
func (s *Service) postUpsert(ctx context.Context, vaultID string, dbInput models.FileInput, contentHash string) (*models.File, error) {
	if err := s.db.EnsureFolders(ctx, vaultID, dbInput.Path); err != nil {
		return nil, fmt.Errorf("ensure parent folders: %w", err)
	}

	doc, created, previousFile, err := s.db.UpsertFile(ctx, dbInput)
	if err != nil {
		return nil, fmt.Errorf("upsert file: %w", err)
	}

	if !created && previousFile != nil {
		fileID, idErr := models.RecordIDString(doc.ID)
		if idErr != nil {
			logutil.FromCtx(ctx).Warn("failed to extract file ID for versioning", "error", idErr)
		} else {
			s.maybeCreateVersion(ctx, fileID, vaultID, previousFile, contentHash)
		}
	}

	if created {
		s.publishFileEvent("file.created", vaultID, doc)
	} else {
		s.publishFileEvent("file.updated", vaultID, doc)
	}

	if err := s.enqueueJob(ctx, doc, created, previousFile); err != nil {
		logutil.FromCtx(ctx).Error("failed to enqueue pipeline job", "path", doc.Path, "error", err)
	}

	return doc, nil
}

// enqueueJob creates the appropriate pipeline job for a newly created or updated file.
// For updates with unchanged content (same content hash), no job is created.
// For updates with changed content, outstanding jobs are cancelled first.
func (s *Service) enqueueJob(ctx context.Context, doc *models.File, created bool, previousFile *models.File) error {
	contentChanged := created ||
		(previousFile != nil && (previousFile.Hash == nil || doc.Hash == nil ||
			*previousFile.Hash != *doc.Hash))

	if !contentChanged {
		return nil
	}

	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return fmt.Errorf("extract file id: %w", err)
	}

	// Cancel outstanding jobs from the previous version.
	if !created {
		if err := s.db.CancelJobsForFile(ctx, fileID); err != nil {
			return fmt.Errorf("cancel old jobs: %w", err)
		}
	}

	isBinary := !models.IsTextFile(doc.Path) && doc.Size > 0

	var jobType string
	if isBinary && models.IsAudioFile(doc.Path) {
		// Always enqueue transcribe even when no transcriber is configured now;
		// the handler skips silently. This ensures files are processed if a
		// transcriber is later enabled via SIGHUP reload.
		jobType = "transcribe"
	} else if isBinary && models.IsPDFFile(doc.Path) {
		// Always enqueue PDF processing — the handler checks for poppler
		// availability and degrades gracefully if not installed.
		jobType = "pdf"
	} else if !isBinary {
		jobType = "parse"
	}
	// Other binary types (images, etc.) have no pipeline job.

	if jobType == "" {
		return nil
	}

	if err := s.db.CreateJob(ctx, fileID, jobType, 0); err != nil {
		return fmt.Errorf("create %s job: %w", jobType, err)
	}
	if s.bus != nil {
		s.bus.Publish(event.ChangeEvent{Type: "job.created"})
	}
	return nil
}

// ProcessFile runs the full processing pipeline for a file synchronously:
// chunks, wiki-links, dangling link resolution, relations, tasks, and external links.
// Used by ProcessAllPending for tests and CLI commands. The async path uses ParseHandler.
func (s *Service) ProcessFile(ctx context.Context, doc *models.File) error {
	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return fmt.Errorf("extract file id: %w", err)
	}

	vaultID, err := models.RecordIDString(doc.Vault)
	if err != nil {
		return fmt.Errorf("extract vault id: %w", err)
	}

	// Template documents are excluded from chunking and search indexing.
	// Only sync labels and mark processed so they remain browsable.
	isTpl, err := s.isTemplatePath(ctx, vaultID, doc.Path)
	if err != nil {
		return fmt.Errorf("check template path: %w", err)
	}
	if isTpl {
		if err := s.db.SyncFileLabels(ctx, fileID, vaultID, doc.Labels); err != nil {
			return fmt.Errorf("sync labels for template %s: %w", doc.Path, err)
		}
		s.publishFileEvent("file.processed", vaultID, doc)
		return nil
	}

	// Skip markdown-specific processing for binary files
	isBinary := !models.IsTextFile(doc.Path) && doc.Size > 0

	if !isBinary {
		content, err := s.ReadFileContent(ctx, doc)
		if err != nil {
			return fmt.Errorf("read content for %s: %w", doc.Path, err)
		}
		parsed := parser.ParseMarkdown(content)

		// 1. Sync chunks (with smart diffing — only re-embed changed chunks)
		if err := s.syncChunks(ctx, fileID, parsed, doc.Labels); err != nil {
			return fmt.Errorf("sync chunks for %s: %w", doc.Path, err)
		}

		// 2. Extract and store wiki-links
		if err := s.processWikiLinks(ctx, fileID, vaultID, parsed.WikiLinks); err != nil {
			return fmt.Errorf("process wiki-links for %s: %w", doc.Path, err)
		}

		// 3. Resolve dangling links that might point to this file
		if err := s.resolveDanglingForFile(ctx, vaultID, doc); err != nil {
			return fmt.Errorf("resolve dangling links for %s: %w", doc.Path, err)
		}

		// 3b. Un-resolve stem-only links if this file made the stem ambiguous
		if err := s.handleStemAmbiguity(ctx, vaultID, doc.Stem); err != nil {
			return fmt.Errorf("handle stem ambiguity for %s: %w", doc.Path, err)
		}

		// 4. Process explicit relates_to from frontmatter
		if err := s.processRelatesTo(ctx, fileID, vaultID, parsed.Frontmatter); err != nil {
			return fmt.Errorf("process relates_to for %s: %w", doc.Path, err)
		}

		// 5.5. Sync tasks (extract checkboxes, diff with DB)
		if err := s.syncTasks(ctx, fileID, vaultID, parsed.Tasks); err != nil {
			return fmt.Errorf("sync tasks for %s: %w", doc.Path, err)
		}

		// 5.6. Extract and store external links
		if err := s.processExternalLinks(ctx, fileID, vaultID, parsed.ExternalLinks); err != nil {
			return fmt.Errorf("process external links for %s: %w", doc.Path, err)
		}
	} else if models.IsAudioFile(doc.Path) {
		// Audio files are processed by the TranscribeHandler pipeline job.
		// The job was already created by enqueueJob at ingest time — nothing to do here.
	}

	// 5. Sync label graph (has_label edges) — applies to all file types
	if err := s.db.SyncFileLabels(ctx, fileID, vaultID, doc.Labels); err != nil {
		return fmt.Errorf("sync labels for %s: %w", doc.Path, err)
	}

	// 6. Cancel the parse job now that processing is complete.
	if err := s.db.CancelJobsForFile(ctx, fileID); err != nil {
		return fmt.Errorf("cancel parse job: %w", err)
	}

	// 7. Enqueue embed job for new chunks (mirrors ParseHandler behaviour).
	if !models.IsAudioFile(doc.Path) && s.getEmbedder() != nil && s.shouldEmbed(ctx, vaultID, doc.Path) {
		if err := s.db.CreateJob(ctx, fileID, "embed", 0); err != nil {
			logutil.FromCtx(ctx).Warn("failed to enqueue embed job after sync process", "path", doc.Path, "error", err)
		} else if s.bus != nil {
			s.bus.Publish(event.ChangeEvent{Type: "job.created"})
		}
	}

	// 8. Publish processed event
	s.publishFileEvent("file.processed", vaultID, doc)

	return nil
}

// ProcessAllPending processes all unprocessed files in a vault synchronously.
// Intended for tests and CLI commands that need immediate processing.
// If vaultID is empty, processes files across all vaults.
func (s *Service) ProcessAllPending(ctx context.Context, vaultID ...string) error {
	var vid string
	if len(vaultID) > 0 {
		vid = vaultID[0]
	}
	for {
		docs, err := s.db.ListUnprocessedFiles(ctx, vid, 100)
		if err != nil {
			return fmt.Errorf("list unprocessed: %w", err)
		}
		if len(docs) == 0 {
			return nil
		}
		for _, doc := range docs {
			if err := s.ProcessFile(ctx, &doc); err != nil {
				return fmt.Errorf("process %s: %w", doc.Path, err)
			}
		}
	}
}

// embeddingTask pairs a chunk ID with its embedding text.
type embeddingTask struct {
	chunkID string
	text    string
}

// storeEmbeddings stores pre-computed embeddings one-by-one.
// Returns the number of successfully stored embeddings.
// Failures are logged; the job retry mechanism handles re-running the embed job.
func (s *Service) storeEmbeddings(ctx context.Context, updates []db.ChunkEmbeddingUpdate) int {
	logger := logutil.FromCtx(ctx)
	stored := 0
	for _, u := range updates {
		if err := s.db.UpdateChunkEmbedding(ctx, u.ID, u.Embedding); err != nil {
			logger.Warn("failed to store chunk embedding", "chunk_id", u.ID, "error", err)
			continue
		}
		stored++
	}
	return stored
}

// embedChunksOneByOne embeds and stores chunks individually as a fallback
// when batch embedding fails.
func (s *Service) embedChunksOneByOne(ctx context.Context, embedder *llm.Embedder, pending []embeddingTask) (int, error) {
	logger := logutil.FromCtx(ctx)
	var updates []db.ChunkEmbeddingUpdate
	var lastErr error
	for _, p := range pending {
		emb, err := embedder.Embed(ctx, p.text)
		if err != nil {
			logger.Warn("failed to embed chunk", "chunk_id", p.chunkID, "error", err)
			lastErr = err
			continue
		}
		updates = append(updates, db.ChunkEmbeddingUpdate{ID: p.chunkID, Embedding: emb})
	}
	stored := s.storeEmbeddings(ctx, updates)
	if stored == 0 && lastErr != nil {
		return 0, fmt.Errorf("all %d chunks failed to embed: %w", len(pending), lastErr)
	}
	return stored, nil
}

// buildEmbeddingContext prepends file and section context to chunk content
// for better embedding quality (contextual retrieval technique).
// The context prefix is only used at embedding time, not stored in the chunk.
// If maxChars > 0 and the assembled string exceeds maxChars, the content is
// truncated at a word boundary (the prefix is preserved).
func buildEmbeddingContext(chunk models.Chunk, fileTitle string, maxChars int) string {
	var b strings.Builder
	if fileTitle != "" {
		fmt.Fprintf(&b, "File: %s\n", fileTitle)
	}
	if chunk.SourceLoc != nil && *chunk.SourceLoc != "" {
		section := stripMarkdownHeadingPrefixes(*chunk.SourceLoc)
		fmt.Fprintf(&b, "Section: %s\n", section)
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	prefixLen := b.Len()

	b.WriteString(chunk.Text)
	result := b.String()

	if maxChars > 0 && len(result) > maxChars {
		if prefixLen > maxChars {
			// Prefix alone exceeds limit — truncate the entire string.
			// This shouldn't happen with a properly sized maxEmbedContextOverhead
			// (see config.maxEmbedContextOverhead), but guard against it.
			result = result[:maxChars]
		} else {
			// Truncate content at word boundary, keeping prefix intact.
			contentBudget := maxChars - prefixLen
			result = result[:prefixLen] + truncateAtWordBoundary(chunk.Text, contentBudget)
		}
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

// EmbedPendingChunksForFile embeds all un-embedded chunks for a specific file.
// Text chunks use the regular Embedder; multimodal chunks (with DataHash and
// non-text MIME type) use the MultimodalEmbedder, falling back to text embedding.
// Returns the number of chunks successfully embedded.
func (s *Service) EmbedPendingChunksForFile(ctx context.Context, fileID string) (int, error) {
	embedder := s.getEmbedder()
	mmEmbedder := s.getMultimodalEmbedder()

	if embedder == nil && mmEmbedder == nil {
		logutil.FromCtx(ctx).Debug("skipping embedding, no embedders configured", "file_id", fileID)
		return 0, nil
	}

	chunks, err := s.db.GetUnembeddedChunks(ctx, fileID)
	if err != nil {
		return 0, fmt.Errorf("get unembedded chunks for file: %w", err)
	}

	// Fetch file title once for contextual embedding (shared by all chunks).
	logger := logutil.FromCtx(ctx)
	fileTitle := ""
	if doc, fetchErr := s.db.GetFileByID(ctx, fileID); fetchErr == nil && doc != nil {
		fileTitle = doc.Title
	}

	// Partition chunks into text and multimodal tasks.
	var textPending []embeddingTask
	var mmPending []multimodalEmbeddingTask

	for _, chunk := range chunks {
		chunkID, err := models.RecordIDString(chunk.ID)
		if err != nil {
			logger.Warn("failed to extract chunk ID for embedding", "error", err)
			continue
		}

		if chunk.IsMultimodal() {
			mmPending = append(mmPending, multimodalEmbeddingTask{
				chunkID:  chunkID,
				dataHash: *chunk.Hash,
				mimeType: chunk.MimeType,
				text:     buildEmbeddingContext(chunk, fileTitle, s.embedMaxInputChars),
			})
		} else {
			textPending = append(textPending, embeddingTask{
				chunkID: chunkID,
				text:    buildEmbeddingContext(chunk, fileTitle, s.embedMaxInputChars),
			})
		}
	}

	var (
		total int
		errs  []error
	)

	// Embed text chunks via regular embedder.
	if len(textPending) > 0 && embedder != nil {
		n, err := s.embedTextChunks(ctx, embedder, textPending)
		if err != nil {
			logger.Warn("text chunk embedding failed", "error", err)
			errs = append(errs, fmt.Errorf("text chunks: %w", err))
		}
		total += n
	}

	// Embed multimodal chunks.
	if len(mmPending) > 0 {
		n, err := s.embedMultimodalChunks(ctx, mmEmbedder, embedder, mmPending)
		if err != nil {
			logger.Warn("multimodal chunk embedding failed", "error", err)
			errs = append(errs, fmt.Errorf("multimodal chunks: %w", err))
		}
		total += n
	}

	return total, errors.Join(errs...)
}

// multimodalEmbeddingTask holds the data needed to embed a multimodal chunk.
type multimodalEmbeddingTask struct {
	chunkID  string
	dataHash string // blob store reference for the binary data
	mimeType string // MIME type of the binary data
	text     string // extracted text (fallback for text-only embedding)
}

// embedTextChunks embeds text chunks using the regular text embedder.
func (s *Service) embedTextChunks(ctx context.Context, embedder *llm.Embedder, pending []embeddingTask) (int, error) {
	logger := logutil.FromCtx(ctx)

	texts := make([]string, len(pending))
	for i, p := range pending {
		texts[i] = p.text
	}

	vectors, batchErr := embedder.EmbedBatch(ctx, texts)
	if batchErr != nil {
		logger.Warn("batch embedding failed, falling back to one-at-a-time", "batch_size", len(texts), "error", batchErr)
		return s.embedChunksOneByOne(ctx, embedder, pending)
	}

	if len(vectors) != len(pending) {
		logger.Error("batch embedding returned wrong vector count", "expected", len(pending), "got", len(vectors))
		return s.embedChunksOneByOne(ctx, embedder, pending)
	}

	updates := make([]db.ChunkEmbeddingUpdate, len(vectors))
	for i, vec := range vectors {
		updates[i] = db.ChunkEmbeddingUpdate{
			ID:        pending[i].chunkID,
			Embedding: vec,
		}
	}

	if err := s.db.BatchUpdateChunkEmbeddings(ctx, updates); err != nil {
		logger.Warn("batch store failed, falling back to individual updates", "error", err)
		return s.storeEmbeddings(ctx, updates), nil
	}

	return len(vectors), nil
}

// embedMultimodalChunks embeds chunks using the MultimodalEmbedder (reading binary
// data from the blob store), or falls back to text embedding if no multimodal
// embedder is configured. Blob reads are batched alongside embedding calls to
// avoid loading all page images into memory at once.
func (s *Service) embedMultimodalChunks(ctx context.Context, mmEmbedder *llm.MultimodalEmbedder, textEmbedder *llm.Embedder, tasks []multimodalEmbeddingTask) (int, error) {
	// Fallback: if no multimodal embedder, use text embedder on chunk.text.
	if mmEmbedder == nil {
		if textEmbedder == nil {
			return 0, nil
		}
		textTasks := make([]embeddingTask, len(tasks))
		for i, t := range tasks {
			textTasks[i] = embeddingTask{chunkID: t.chunkID, text: t.text}
		}
		return s.embedTextChunks(ctx, textEmbedder, textTasks)
	}

	logger := logutil.FromCtx(ctx)
	embedder := *mmEmbedder

	// Process in batches to avoid loading all page images into memory.
	const batchSize = 6 // matches geminiEmbedBatchLimit
	var (
		total     int
		errs      []error
		failedIDs []string
	)

	for offset := 0; offset < len(tasks); offset += batchSize {
		end := min(offset+batchSize, len(tasks))
		batch := tasks[offset:end]

		// Read binary data from blob store for this batch.
		var inputs []llm.MultimodalInput
		var chunkIDs []string

		for _, task := range batch {
			rc, err := s.blobStore.Get(ctx, task.dataHash)
			if err != nil {
				errs = append(errs, fmt.Errorf("read blob %s (chunk %s): %w", task.dataHash, task.chunkID, err))
				failedIDs = append(failedIDs, task.chunkID)
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				errs = append(errs, fmt.Errorf("read blob data %s (chunk %s): %w", task.dataHash, task.chunkID, err))
				failedIDs = append(failedIDs, task.chunkID)
				continue
			}

			inputs = append(inputs, llm.MultimodalInput{
				Data:     data,
				MimeType: task.mimeType,
			})
			chunkIDs = append(chunkIDs, task.chunkID)
		}

		if len(inputs) == 0 {
			continue
		}

		vectors, err := embedder.EmbedMultimodal(ctx, inputs)
		if err != nil {
			errs = append(errs, fmt.Errorf("multimodal embedding batch [%d:%d]: %w", offset, end, err))
			continue
		}

		if len(vectors) != len(inputs) {
			errs = append(errs, fmt.Errorf("multimodal embedding returned wrong vector count: expected %d, got %d", len(inputs), len(vectors)))
			continue
		}

		updates := make([]db.ChunkEmbeddingUpdate, len(vectors))
		for j, vec := range vectors {
			f32 := make([]float32, len(vec))
			for k, v := range vec {
				f32[k] = float32(v)
			}
			updates[j] = db.ChunkEmbeddingUpdate{
				ID:        chunkIDs[j],
				Embedding: f32,
			}
		}

		if err := s.db.BatchUpdateChunkEmbeddings(ctx, updates); err != nil {
			errs = append(errs, fmt.Errorf("store multimodal embeddings: %w", err))
			continue
		}

		total += len(vectors)
	}

	if len(failedIDs) > 0 {
		logger.Warn("some blobs could not be read for multimodal embedding",
			"failed_count", len(failedIDs), "failed_chunk_ids", failedIDs, "succeeded", total)
	}

	return total, errors.Join(errs...)
}

// Get returns a file by vault+path, or nil if not found.
func (s *Service) Get(ctx context.Context, vaultID, path string) (*models.File, error) {
	return s.db.GetFileByPath(ctx, vaultID, path)
}

// GetMeta returns lightweight file metadata, or nil if not found.
func (s *Service) GetMeta(ctx context.Context, vaultID, path string) (*models.FileMeta, error) {
	return s.db.GetFileMetaByPath(ctx, vaultID, path)
}

// ListMetas returns lightweight metadata for files matching a filter.
func (s *Service) ListMetas(ctx context.Context, filter db.ListFilesFilter) ([]models.FileMeta, error) {
	return s.db.ListFileMetas(ctx, filter)
}

// Update re-runs the file pipeline on existing content.
func (s *Service) Update(ctx context.Context, vaultID, path, content string) (*models.File, error) {
	return s.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    path,
		Content: content,
	})
}

// Delete removes a file and all its associated data.
func (s *Service) Delete(ctx context.Context, vaultID, path string) error {
	path = models.NormalizePath(path)
	doc, err := s.db.GetFileByPath(ctx, vaultID, path)
	if err != nil {
		return fmt.Errorf("get file: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("file not found: %s", path)
	}

	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return fmt.Errorf("extract file id: %w", err)
	}

	var contentHash string
	if doc.Hash != nil {
		contentHash = *doc.Hash
	}

	// Atomic cleanup: wiki links, label edges, chunks, and file record in one transaction.
	// Async cascade events remain as a safety net.
	if err := s.db.DeleteFileAtomic(ctx, fileID); err != nil {
		return fmt.Errorf("delete file atomic: %w", err)
	}

	// After deletion, check if this file's stem is now unique (collision removed).
	// This must happen after DeleteFileAtomic so CountFilesByStem returns the correct count.
	stem := models.FilenameStem(doc.Path)
	if err := s.resolveAfterStemCollisionRemoval(ctx, vaultID, stem); err != nil {
		logutil.FromCtx(ctx).Warn("resolve after stem collision removal", "stem", stem, "error", err)
	}

	// Clean up blob data. Content-addressed blobs may be shared across files
	// (dedup), but in practice hash collisions across different files are rare.
	// A proper GC would require reference counting; for now, best-effort delete.
	if contentHash != "" {
		if err := s.blobStore.Delete(ctx, contentHash); err != nil {
			logutil.FromCtx(ctx).Warn("failed to delete blob", "hash", contentHash, "error", err)
		}
	}

	s.publishFileDeleteEvent(vaultID, fileID, path, contentHash)

	return nil
}

// DeleteByPrefix removes all files whose path starts with the given prefix.
// Returns the number of deleted files. The prefix is normalized and ensured
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
	count, err := s.db.DeleteFilesByPrefix(ctx, vaultID, prefix)
	if err != nil {
		return 0, fmt.Errorf("delete by prefix: %w", err)
	}

	// Clean up folder records under the same prefix
	if _, err := s.db.DeleteFoldersByPrefix(ctx, vaultID, prefix); err != nil {
		return count, fmt.Errorf("delete folder records: %w", err)
	}

	return count, nil
}

// MoveByPrefix renames all files whose path starts with oldPrefix,
// replacing oldPrefix with newPrefix. Returns the number of moved files.
// Both prefixes are normalized and ensured to end with "/" to avoid partial matches.
//
// Recomputes stems and incoming wiki-link raw_targets for all moved files.
// Does not update doc_relations referencing the old paths.
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
	// Atomic: move files, recompute stems, move folders, ensure ancestors in one transaction
	count, err := s.db.MoveByPrefixAtomic(ctx, vaultID, oldNorm, newNorm)
	if err != nil {
		return 0, fmt.Errorf("move by prefix: %w", err)
	}

	// Post-commit best-effort: raw_target recomputation is cosmetic (affects display only)
	movedFiles, err := s.db.ListFilesByPrefix(ctx, vaultID, newNorm)
	if err != nil {
		logutil.FromCtx(ctx).Warn("list moved files for raw target recompute", "error", err)
	} else {
		for _, f := range movedFiles {
			fID, err := models.RecordIDString(f.ID)
			if err != nil {
				logutil.FromCtx(ctx).Warn("extract file id for raw target recompute", "error", err)
				continue
			}
			if err := s.recomputeIncomingRawTargets(ctx, vaultID, fID, f.Path); err != nil {
				logutil.FromCtx(ctx).Warn("recompute incoming raw targets after prefix move", "path", f.Path, "error", err)
			}
		}
	}

	return count, nil
}

// Move changes a file's path.
func (s *Service) Move(ctx context.Context, vaultID, oldPath, newPath string) (*models.File, error) {
	oldPath = models.NormalizePath(oldPath)
	doc, err := s.db.GetFileByPath(ctx, vaultID, oldPath)
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}
	if doc == nil {
		return nil, fmt.Errorf("file not found: %s", oldPath)
	}

	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return nil, fmt.Errorf("extract file id: %w", err)
	}

	oldStem := models.FilenameStem(oldPath)
	normalizedNew := models.NormalizePath(newPath)
	newStem := models.FilenameStem(normalizedNew)

	// Atomic: update file path + ensure destination folders in one transaction
	doc, err = s.db.MoveFileAtomic(ctx, vaultID, fileID, normalizedNew)
	if err != nil {
		return nil, fmt.Errorf("move file: %w", err)
	}

	// Post-commit best-effort: raw_target recomputation is cosmetic (affects display only)
	if err := s.recomputeIncomingRawTargets(ctx, vaultID, fileID, normalizedNew); err != nil {
		logutil.FromCtx(ctx).Warn("recompute incoming raw targets", "error", err)
	}

	// Handle ambiguity for the new stem
	if err := s.handleStemAmbiguity(ctx, vaultID, newStem); err != nil {
		return nil, fmt.Errorf("handle stem ambiguity for new stem: %w", err)
	}

	// If stem changed, check if old stem now has exactly 1 file → resolve dangling links for it
	if oldStem != newStem {
		if err := s.resolveAfterStemCollisionRemoval(ctx, vaultID, oldStem); err != nil {
			logutil.FromCtx(ctx).Warn("resolve after stem collision removal", "stem", oldStem, "error", err)
		}
	}

	s.publishFileMoveEvent(vaultID, doc, oldPath)

	return doc, nil
}

func (s *Service) processWikiLinks(ctx context.Context, fileID, vaultID string, targets []string) error {
	logger := logutil.FromCtx(ctx)

	// Delete existing links from this file
	if err := s.db.DeleteWikiLinks(ctx, fileID); err != nil {
		return fmt.Errorf("delete old wiki-links: %w", err)
	}

	if len(targets) == 0 {
		return nil
	}

	links := make([]db.WikiLinkInput, 0, len(targets))
	for _, target := range targets {
		var toFileID *string
		resolved, err := s.resolver.Resolve(ctx, vaultID, target)
		if err != nil {
			logger.Warn("failed to resolve wiki-link", "target", target, "error", err)
		} else if resolved != nil {
			id, err := models.RecordIDString(resolved.ID)
			if err != nil {
				logger.Warn("failed to extract resolved file ID for wiki-link", "target", target, "error", err)
			} else {
				toFileID = &id
			}
		}
		links = append(links, db.WikiLinkInput{
			RawTarget: target,
			ToFileID:  toFileID,
		})
	}

	return s.db.CreateWikiLinks(ctx, fileID, vaultID, links)
}

func (s *Service) resolveDanglingForFile(ctx context.Context, vaultID string, doc *models.File) error {
	logger := logutil.FromCtx(ctx)
	if doc.Stem == "" {
		return nil
	}
	fileID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return fmt.Errorf("extract file id: %w", err)
	}
	count, err := s.db.CountFilesByStem(ctx, vaultID, doc.Stem)
	if err != nil {
		return fmt.Errorf("count files by stem: %w", err)
	}
	if count != 1 {
		return nil // ambiguous
	}
	n, err := s.db.ResolveDanglingLinksByStem(ctx, vaultID, doc.Stem, fileID)
	if err != nil {
		return fmt.Errorf("resolve dangling by stem: %w", err)
	}
	if n > 0 {
		logger.Info("resolved dangling wiki-links by stem", "stem", doc.Stem, "count", n)
	}
	return nil
}

// resolveAfterStemCollisionRemoval checks if a stem now has exactly one file
// (after a delete or rename removed a collision) and resolves dangling stem-only
// links to that remaining file.
func (s *Service) resolveAfterStemCollisionRemoval(ctx context.Context, vaultID, stem string) error {
	if stem == "" {
		return nil
	}
	remaining, err := s.db.GetFilesByStem(ctx, vaultID, stem)
	if err != nil {
		return fmt.Errorf("get files by stem: %w", err)
	}
	if len(remaining) != 1 {
		return nil
	}
	remainingID, err := models.RecordIDString(remaining[0].ID)
	if err != nil {
		return fmt.Errorf("extract remaining file id: %w", err)
	}
	n, err := s.db.ResolveDanglingLinksByStem(ctx, vaultID, stem, remainingID)
	if err != nil {
		return fmt.Errorf("resolve dangling links by stem: %w", err)
	}
	if n > 0 {
		logutil.FromCtx(ctx).Info("resolved dangling links after stem collision removal", "stem", stem, "count", n)
	}
	return nil
}

// handleStemAmbiguity checks if a stem is now ambiguous (multiple files share it)
// and un-resolves any stem-only wiki-links that pointed to files with that stem.
func (s *Service) handleStemAmbiguity(ctx context.Context, vaultID, stem string) error {
	if stem == "" {
		return nil
	}
	logger := logutil.FromCtx(ctx)
	count, err := s.db.CountFilesByStem(ctx, vaultID, stem)
	if err != nil {
		return fmt.Errorf("count files by stem: %w", err)
	}
	if count <= 1 {
		return nil
	}
	n, err := s.db.UnresolveStemOnlyLinks(ctx, vaultID, stem)
	if err != nil {
		return fmt.Errorf("unresolve ambiguous stem links: %w", err)
	}
	if n > 0 {
		logger.Info("un-resolved ambiguous stem-only wiki-links", "stem", stem, "count", n)
	}
	return nil
}

// recomputeIncomingRawTargets updates the raw_target of all wiki-links pointing
// to a file so they use the shortest unambiguous target for the file's current path.
func (s *Service) recomputeIncomingRawTargets(ctx context.Context, vaultID, fileID, filePath string) error {
	logger := logutil.FromCtx(ctx)
	stem := models.FilenameStem(filePath)
	if stem == "" {
		return nil
	}
	sameStems, err := s.db.GetFilesByStem(ctx, vaultID, stem)
	if err != nil {
		return fmt.Errorf("get files by stem: %w", err)
	}
	newTarget := ShortestUnambiguousTarget(filePath, sameStems)
	links, err := s.db.GetWikiLinksToFile(ctx, fileID)
	if err != nil {
		return fmt.Errorf("get wiki links to file: %w", err)
	}
	var toUpdate []string
	for _, link := range links {
		if link.RawTarget == newTarget {
			continue
		}
		linkID, err := models.RecordIDString(link.ID)
		if err != nil {
			logger.Warn("failed to extract link ID", "error", err)
			continue
		}
		toUpdate = append(toUpdate, linkID)
	}
	if err := s.db.BatchUpdateWikiLinkRawTargets(ctx, toUpdate, newTarget); err != nil {
		logger.Warn("failed to batch update raw targets", "error", err)
	}
	return nil
}

// syncChunks performs smart chunk diffing: compares new chunks against existing ones
// by content, preserving embeddings for unchanged chunks and scheduling embedding
// only for new/changed chunks.
func (s *Service) syncChunks(ctx context.Context, fileID string, parsed *parser.MarkdownDoc, labels []string) error {
	logger := logutil.FromCtx(ctx)
	newChunkResults := parser.ChunkMarkdown(parsed, s.chunkConfig)

	oldChunks, err := s.db.GetChunks(ctx, fileID)
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
			logger.Warn("failed to extract chunk ID during sync", "error", err)
			continue
		}
		allOldIDs = append(allOldIDs, id)
		// Only store first occurrence per content (handles duplicates)
		if _, exists := oldByContent[c.Text]; !exists {
			oldByContent[c.Text] = &oldChunkEntry{chunk: c, id: id}
		}
	}

	matchedOldIDs := make(map[string]bool)
	var toCreate []models.ChunkInput
	var positionUpdates []db.ChunkPositionUpdate

	for i, newChunk := range newChunkResults {
		if entry, ok := oldByContent[newChunk.Content]; ok && !matchedOldIDs[entry.id] {
			// Content unchanged — keep existing chunk (preserve embedding)
			matchedOldIDs[entry.id] = true
			// Update position if it changed
			if entry.chunk.Position != i {
				positionUpdates = append(positionUpdates, db.ChunkPositionUpdate{ID: entry.id, Position: i})
			}
		} else {
			// New or changed chunk — create with embed_at
			var headingPath *string
			if newChunk.HeadingPath != "" {
				headingPath = &newChunk.HeadingPath
			}

			input := models.ChunkInput{
				FileID:    fileID,
				Text:      newChunk.Content,
				Position:  i,
				SourceLoc: headingPath,
				Labels:    labels,
				MimeType:  "text/plain",
				Embedding: nil, // nil until embed job fills it
			}

			toCreate = append(toCreate, input)
		}
	}

	// Batch update positions for reordered chunks
	if err := s.db.BatchUpdateChunkPositions(ctx, positionUpdates); err != nil {
		logger.Warn("failed to batch update chunk positions", "error", err)
	}

	// Batch delete old chunks that were not matched (removed content)
	var toDelete []string
	for _, id := range allOldIDs {
		if !matchedOldIDs[id] {
			toDelete = append(toDelete, id)
		}
	}
	if err := s.db.DeleteChunksByIDs(ctx, toDelete); err != nil {
		logger.Warn("failed to batch delete removed chunks", "error", err)
	}

	// Create new chunks
	if len(toCreate) > 0 {
		if err := s.db.CreateChunks(ctx, toCreate); err != nil {
			return fmt.Errorf("create new chunks: %w", err)
		}
	}

	return nil
}

func (s *Service) processRelatesTo(ctx context.Context, fileID, vaultID string, frontmatter map[string]any) error {
	// Delete existing frontmatter-derived relations before recreating
	if err := s.db.DeleteRelationsBySource(ctx, fileID, string(models.RelSourceFrontmatter)); err != nil {
		return fmt.Errorf("delete old frontmatter relations: %w", err)
	}

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

	var inputs []models.FileRelationInput
	for _, target := range targets {
		resolved, err := s.resolver.Resolve(ctx, vaultID, target)
		if err != nil {
			return fmt.Errorf("resolve relates_to target %q: %w", target, err)
		}
		if resolved == nil {
			logutil.FromCtx(ctx).Info("relates_to target not found", "target", target)
			continue
		}
		toFileID, err := models.RecordIDString(resolved.ID)
		if err != nil {
			return fmt.Errorf("extract resolved file id for %q: %w", target, err)
		}
		inputs = append(inputs, models.FileRelationInput{
			FromFileID: fileID,
			ToFileID:   toFileID,
			RelType:    string(models.RelRelatesTo),
			Source:     string(models.RelSourceFrontmatter),
		})
	}
	if err := s.db.BatchCreateRelations(ctx, inputs); err != nil {
		return fmt.Errorf("create relates_to relations: %w", err)
	}

	return nil
}

func (s *Service) mergeTranscriptionParts(ctx context.Context, transcriber stt.Transcriber, parts []stt.SplitPart, mimeType string) (*stt.Result, error) {
	var allSegments []stt.Segment
	var fullText strings.Builder
	for i, part := range parts {
		partResult, err := transcriber.Transcribe(ctx, part.Data, mimeType)
		if err != nil {
			return nil, fmt.Errorf("transcribe part %d: %w", i, err)
		}
		if fullText.Len() > 0 {
			fullText.WriteString(" ")
		}
		fullText.WriteString(partResult.Text)
		for _, seg := range partResult.Segments {
			allSegments = append(allSegments, stt.Segment{
				Start: seg.Start + part.OffsetSecs,
				End:   seg.End + part.OffsetSecs,
				Text:  seg.Text,
			})
		}
	}
	return &stt.Result{Text: fullText.String(), Segments: allSegments}, nil
}

func (s *Service) transcribeFile(ctx context.Context, transcriber stt.Transcriber, f *models.File, fileID string) error {
	logger := logutil.FromCtx(ctx).With("path", f.Path, "file_id", fileID)

	if f.Hash == nil {
		return fmt.Errorf("file has no content hash")
	}

	var result *stt.Result
	var err error

	if local, ok := s.blobStore.(blob.LocalPathStore); ok {
		blobPath := local.LocalPath(*f.Hash)
		if f.Size > stt.MaxWhisperFileSize {
			parts, splitErr := stt.SplitForTranscriptionFromPath(ctx, blobPath, f.MimeType, stt.MaxWhisperFileSize)
			if splitErr != nil {
				return fmt.Errorf("split audio: %w", splitErr)
			}
			result, err = s.mergeTranscriptionParts(ctx, transcriber, parts, f.MimeType)
		} else {
			data, readErr := os.ReadFile(blobPath)
			if readErr != nil {
				return fmt.Errorf("read blob: %w", readErr)
			}
			result, err = transcriber.Transcribe(ctx, data, f.MimeType)
		}
	} else {
		rc, getErr := s.blobStore.Get(ctx, *f.Hash)
		if getErr != nil {
			return fmt.Errorf("get blob: %w", getErr)
		}
		data, readErr := io.ReadAll(rc)
		rc.Close()
		if readErr != nil {
			return fmt.Errorf("read blob: %w", readErr)
		}
		if len(data) > stt.MaxWhisperFileSize {
			parts, splitErr := stt.SplitForTranscription(ctx, data, f.MimeType, stt.MaxWhisperFileSize)
			if splitErr != nil {
				return fmt.Errorf("split audio: %w", splitErr)
			}
			result, err = s.mergeTranscriptionParts(ctx, transcriber, parts, f.MimeType)
		} else {
			result, err = transcriber.Transcribe(ctx, data, f.MimeType)
		}
	}
	if err != nil {
		return fmt.Errorf("transcribe: %w", err)
	}

	logger.Info("transcription complete", "segments", len(result.Segments), "text_len", len(result.Text))

	// Group segments into time-window chunks
	chunks := pipeline.GroupSegments(result.Segments, s.audioSegmentSeconds)

	// Delete any existing chunks for this file (re-transcription case)
	if err := s.db.DeleteChunks(ctx, fileID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}

	// Create text chunks with embedding scheduled
	var chunkInputs []models.ChunkInput
	for _, chunk := range chunks {
		chunkInputs = append(chunkInputs, models.ChunkInput{
			FileID:    fileID,
			Text:      chunk.Text,
			Position:  chunk.Position,
			SourceLoc: &chunk.SourceLoc,
			Labels:    f.Labels,
			MimeType:  "text/plain",
		})
	}

	if len(chunkInputs) > 0 {
		if err := s.db.CreateChunks(ctx, chunkInputs); err != nil {
			return fmt.Errorf("create chunks: %w", err)
		}
	}

	// Store transcript in blob and update DB metadata.
	if err := s.storeTranscript(ctx, fileID, result.Text); err != nil {
		return fmt.Errorf("store transcript: %w", err)
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
