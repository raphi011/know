package document

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// ProcessingWorker polls for unprocessed documents and runs the deferred
// pipeline (chunks, wiki-links, relations). Uses WorkerLoop for the
// run/restart/tick pattern. When an event bus is provided, also wakes
// immediately on document.created/document.updated events.
type ProcessingWorker struct {
	service    *Service
	batch      int
	interval   time.Duration
	bus        *event.Bus
	failures   map[string]int // docID → consecutive failure count
	maxRetries int
}

// NewProcessingWorker creates a worker that polls for unprocessed documents.
// If bus is non-nil, the worker also wakes immediately on document create/update events.
// Panics if service is nil, interval <= 0, or batchSize <= 0 (programmer errors).
func NewProcessingWorker(service *Service, interval time.Duration, batchSize int, bus *event.Bus) *ProcessingWorker {
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
		batch:      batchSize,
		interval:   interval,
		bus:        bus,
		failures:   make(map[string]int),
		maxRetries: 5,
	}
}

// Run starts the processing worker loop. It blocks until the context is cancelled.
// Subscribes to bus events here (not in constructor) to avoid goroutine leaks
// if the worker is constructed but never started.
func (w *ProcessingWorker) Run(ctx context.Context) {
	notify, unsub := eventNotify(w.bus, "document.created", "document.updated")
	defer unsub()
	loop := NewWorkerLoop("document processing worker", w.interval, w.tick, notify)
	loop.Run(ctx)
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

		if err := w.processOne(ctx, docID, &doc); err != nil {
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

func (w *ProcessingWorker) processOne(ctx context.Context, docID string, doc *models.Document) error {
	// Re-fetch to get the latest version (another write may have happened)
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
