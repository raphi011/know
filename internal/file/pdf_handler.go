package file

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/pipeline"
)

// PDFHandler returns a pipeline.Handler that processes PDF files:
// renders pages as PNGs, extracts text via LLM (or pdftotext fallback),
// stores page images in the blob store, and creates chunks.
// Returns an error if poppler is not installed so the job stays in the queue.
func PDFHandler(svc *Service, bus *event.Bus) pipeline.Handler {
	return func(ctx context.Context, job models.PipelineJob) error {
		logger := logutil.FromCtx(ctx)

		fileID, err := models.RecordIDString(job.File)
		if err != nil {
			return fmt.Errorf("extract file id: %w", err)
		}

		doc, err := svc.db.GetFileByID(ctx, fileID)
		if err != nil {
			return fmt.Errorf("get file: %w", err)
		}
		if doc == nil {
			logger.Warn("pdf job references missing file, skipping", "file_id", fileID)
			return nil
		}

		logger = logger.With("path", doc.Path, "file_id", fileID)

		// Check poppler availability — return error so the job stays in the
		// queue and can be retried after installing poppler.
		if err := pipeline.CheckPoppler(); err != nil {
			return fmt.Errorf("poppler not installed (install with: brew install poppler): %w", err)
		}

		if err := svc.processPDF(ctx, doc, fileID); err != nil {
			return fmt.Errorf("process pdf: %w", err)
		}

		vaultID, err := models.RecordIDString(doc.Vault)
		if err != nil {
			logger.Warn("failed to extract vault id after pdf processing", "file_id", fileID, "error", err)
		}

		// Enqueue embed job if any embedder is configured and folder allows embedding.
		if (svc.getEmbedder() != nil || svc.getMultimodalEmbedder() != nil) && (vaultID == "" || svc.shouldEmbed(ctx, vaultID, doc.Path)) {
			if err := svc.db.CreateJob(ctx, fileID, "embed", 0); err != nil {
				return fmt.Errorf("create embed job for %s: %w", doc.Path, err)
			}
			if bus != nil {
				bus.Publish(event.ChangeEvent{Type: "job.created"})
			}
		}

		if vaultID != "" {
			svc.publishFileEvent("file.processed", vaultID, doc)
		}
		return nil
	}
}

// processPDF renders each page of a PDF as PNG, extracts text, stores page
// images in the blob store, and creates one chunk per page.
func (s *Service) processPDF(ctx context.Context, f *models.File, fileID string) error {
	logger := logutil.FromCtx(ctx).With("path", f.Path)

	if f.ContentHash == nil {
		return fmt.Errorf("file has no content hash")
	}

	// Resolve PDF to a local file path for poppler CLI tools.
	pdfPath, cleanup, err := s.resolveBlobPath(ctx, f)
	if err != nil {
		return fmt.Errorf("resolve blob path: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	// Create temp directory for rendered PNGs.
	tmpDir, err := os.MkdirTemp("", "know-pdf-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Render all pages as PNGs.
	dpi := s.pdfRenderDPI
	if dpi <= 0 {
		dpi = 300
	}
	pngPaths, err := pipeline.RenderPages(ctx, pdfPath, dpi, tmpDir)
	if err != nil {
		return fmt.Errorf("render pages: %w", err)
	}

	logger.Info("rendered pdf pages", "page_count", len(pngPaths))

	// Get labels for chunks.
	labels := f.Labels
	if labels == nil {
		labels = []string{}
	}

	// Process each page: extract text + store PNG.
	var chunks []models.ChunkInput
	var allText strings.Builder

	textExtractor := s.getTextExtractor()

	for i, pngPath := range pngPaths {
		pageNum := i + 1

		// Read PNG bytes.
		pngData, err := os.ReadFile(pngPath)
		if err != nil {
			return fmt.Errorf("read page %d png: %w", pageNum, err)
		}

		// Store PNG in blob store.
		pngHash := sha256Hex(pngData)
		if err := s.blobStore.Put(ctx, pngHash, bytes.NewReader(pngData), int64(len(pngData))); err != nil {
			return fmt.Errorf("store page %d png: %w", pageNum, err)
		}

		// Extract text: try LLM first, fall back to pdftotext.
		text := s.extractPageText(ctx, textExtractor, pngData, pdfPath, pageNum)

		if allText.Len() > 0 {
			allText.WriteString("\n\n")
		}
		allText.WriteString(text)

		sourceLoc := fmt.Sprintf("Page %d", pageNum)
		chunks = append(chunks, models.ChunkInput{
			FileID:    fileID,
			Text:      text,
			MimeType:  "image/png",
			Position:  pageNum,
			SourceLoc: &sourceLoc,
			DataHash:  &pngHash,
			Labels:    labels,
		})
	}

	// Delete existing chunks and create new ones.
	if err := s.db.DeleteChunks(ctx, fileID); err != nil {
		return fmt.Errorf("delete old chunks: %w", err)
	}
	if err := s.db.CreateChunks(ctx, chunks); err != nil {
		return fmt.Errorf("create chunks: %w", err)
	}

	// Store concatenated extracted text in blob and update DB metadata.
	if allText.Len() > 0 {
		text := allText.String()
		textHash := models.ContentHash(text)
		if err := s.blobStore.Put(ctx, textHash, strings.NewReader(text), int64(len(text))); err != nil {
			return fmt.Errorf("store pdf text blob: %w", err)
		}
		if err := s.db.UpdateFileTranscript(ctx, fileID, textHash, len(text)); err != nil {
			return fmt.Errorf("update file transcript: %w", err)
		}
	}

	logger.Info("pdf processing complete", "chunks", len(chunks))
	return nil
}

// extractPageText tries the LLM TextExtractor first, falling back to pdftotext.
func (s *Service) extractPageText(ctx context.Context, extractor *llm.TextExtractor, pngData []byte, pdfPath string, pageNum int) string {
	logger := logutil.FromCtx(ctx)

	// Try LLM text extraction from page image.
	if extractor != nil {
		text, err := (*extractor).Extract(ctx, pngData, "image/png")
		if err != nil {
			logger.Warn("llm text extraction failed, falling back to pdftotext",
				"page", pageNum, "error", err)
		} else if text != "" {
			return text
		} else {
			logger.Debug("llm text extraction returned empty, falling back to pdftotext",
				"page", pageNum)
		}
	}

	// Fallback: extract raw text with pdftotext.
	text, err := pipeline.ExtractPageText(ctx, pdfPath, pageNum)
	if err != nil {
		logger.Warn("pdftotext extraction failed", "page", pageNum, "error", err)
		return fmt.Sprintf("[Page %d: text extraction failed]", pageNum)
	}
	return strings.TrimSpace(text)
}

// resolveBlobPath returns a local file path for a blob.
// If the blob store supports local paths, returns it directly.
// Otherwise, downloads to a temp file and returns a cleanup function.
func (s *Service) resolveBlobPath(ctx context.Context, f *models.File) (string, func(), error) {
	if local, ok := s.blobStore.(blob.LocalPathStore); ok {
		return local.LocalPath(*f.ContentHash), nil, nil
	}

	// Download to temp file for S3-backed stores.
	rc, err := s.blobStore.Get(ctx, *f.ContentHash)
	if err != nil {
		return "", nil, fmt.Errorf("get blob: %w", err)
	}
	defer rc.Close()

	ext := filepath.Ext(f.Path)
	tmp, err := os.CreateTemp("", "know-blob-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("copy blob to temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}

	cleanup := func() { os.Remove(tmp.Name()) }
	return tmp.Name(), cleanup, nil
}

// sha256Hex returns the hex-encoded SHA256 hash of the given data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
