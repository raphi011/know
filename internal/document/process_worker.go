package document

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// ProcessingWorker polls for unprocessed documents and runs the deferred
// pipeline (chunks, wiki-links, relations). Mirrors the EmbeddingWorker pattern.
type ProcessingWorker struct {
	service    *Service
	interval   time.Duration
	batch      int
	failures   map[string]int // docID → consecutive failure count
	maxRetries int
}

// NewProcessingWorker creates a worker that polls for unprocessed documents.
// Panics if service is nil, interval <= 0, or batchSize <= 0 (programmer errors).
func NewProcessingWorker(service *Service, interval time.Duration, batchSize int) *ProcessingWorker {
	if service == nil {
		panic("ProcessingWorker: nil service")
	}
	if interval <= 0 {
		panic("ProcessingWorker: interval must be positive")
	}
	if batchSize <= 0 {
		panic("ProcessingWorker: batchSize must be positive")
	}
	return &ProcessingWorker{
		service:    service,
		interval:   interval,
		batch:      batchSize,
		failures:   make(map[string]int),
		maxRetries: 5,
	}
}

// Run starts the processing worker loop. It blocks until the context is cancelled.
// If the worker panics, it logs the stack trace and restarts after a short delay.
func (w *ProcessingWorker) Run(ctx context.Context) {
	logutil.FromCtx(ctx).Info("document processing worker started", "interval", w.interval, "batch_size", w.batch)

	for {
		stopped := w.runLoop(ctx)
		if stopped {
			return
		}
		select {
		case <-ctx.Done():
			logutil.FromCtx(ctx).Info("document processing worker stopped")
			return
		case <-time.After(5 * time.Second):
			logutil.FromCtx(ctx).Info("document processing worker restarting after panic")
		}
	}
}

func (w *ProcessingWorker) runLoop(ctx context.Context) (stopped bool) {
	defer func() {
		if p := recover(); p != nil {
			logutil.FromCtx(ctx).Error("document processing worker panicked",
				"error", p, "stack", string(debug.Stack()))
			stopped = false
		}
	}()

	// Process any pending documents immediately on startup
	w.tick(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logutil.FromCtx(ctx).Info("document processing worker stopped")
			return true
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *ProcessingWorker) tick(ctx context.Context) {
	docs, err := w.service.db.ListUnprocessedDocuments(ctx, w.batch)
	if err != nil {
		logutil.FromCtx(ctx).Error("document processing worker: list unprocessed", "error", err)
		return
	}

	if len(docs) == 0 {
		return
	}

	processed := 0
	for _, doc := range docs {
		if ctx.Err() != nil {
			return // shutdown requested — stop processing
		}

		docID, err := models.RecordIDString(doc.ID)
		if err != nil {
			logutil.FromCtx(ctx).Warn("failed to extract doc ID in processing tick", "path", doc.Path, "error", err)
			continue
		}

		if w.failures[docID] >= w.maxRetries {
			logutil.FromCtx(ctx).Warn("skipping poison-pill document", "path", doc.Path, "id", docID, "failures", w.failures[docID])
			continue
		}

		if err := w.processOne(ctx, &doc); err != nil {
			w.failures[docID]++
			level := slog.LevelWarn
			if w.failures[docID] >= w.maxRetries {
				level = slog.LevelError
			}
			logutil.FromCtx(ctx).Log(ctx, level, "failed to process document",
				"path", doc.Path, "attempt", w.failures[docID],
				"max_retries", w.maxRetries, "error", err)
			continue
		}

		delete(w.failures, docID) // success — clear failure count
		processed++
	}

	if processed > 0 {
		logutil.FromCtx(ctx).Info("document processing worker processed documents", "count", processed)
	}
}

func (w *ProcessingWorker) processOne(ctx context.Context, doc *models.Document) error {
	// Re-fetch to get the latest version (another write may have happened)
	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		return fmt.Errorf("extract document ID: %w", err)
	}

	latest, err := w.service.db.GetDocumentByID(ctx, docID)
	if err != nil {
		return fmt.Errorf("process document %s: %w", docID, err)
	}
	if latest == nil {
		// Document was deleted between listing and processing — skip
		return nil
	}
	if latest.Processed {
		// Already processed (e.g. by a concurrent tick) — skip
		return nil
	}

	return w.service.ProcessDocument(ctx, latest)
}
