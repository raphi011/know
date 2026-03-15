package file

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

// ProcessingWorker polls for unprocessed files and runs the deferred
// pipeline (chunks, wiki-links, relations). Uses WorkerLoop for the
// run/restart/tick pattern. When an event bus is provided, also wakes
// immediately on file.created/file.updated events.
type ProcessingWorker struct {
	service    *Service
	batch      int
	interval   time.Duration
	bus        *event.Bus
	failures   map[string]int // fileID → consecutive failure count
	maxRetries int
}

// NewProcessingWorker creates a worker that polls for unprocessed files.
// If bus is non-nil, the worker also wakes immediately on file create/update events.
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
	notify, unsub := eventNotify(w.bus, "file.created", "file.updated")
	defer unsub()
	loop := NewWorkerLoop("file processing worker", w.interval, w.tick, notify)
	loop.Run(ctx)
}

func (w *ProcessingWorker) tick(ctx context.Context) {
	docs, err := w.service.db.ListUnprocessedFiles(ctx, w.batch)
	if err != nil {
		logutil.FromCtx(ctx).Error("file processing worker: list unprocessed", "error", err)
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

		fileID, err := models.RecordIDString(doc.ID)
		if err != nil {
			logutil.FromCtx(ctx).Warn("failed to extract file ID in processing tick", "path", doc.Path, "error", err)
			continue
		}

		if w.failures[fileID] >= w.maxRetries {
			logutil.FromCtx(ctx).Warn("skipping poison-pill file", "path", doc.Path, "id", fileID, "failures", w.failures[fileID])
			continue
		}

		if err := w.processOne(ctx, fileID, &doc); err != nil {
			w.failures[fileID]++
			level := slog.LevelWarn
			if w.failures[fileID] >= w.maxRetries {
				level = slog.LevelError
			}
			logutil.FromCtx(ctx).Log(ctx, level, "failed to process file",
				"path", doc.Path, "attempt", w.failures[fileID],
				"max_retries", w.maxRetries, "error", err)
			continue
		}

		delete(w.failures, fileID) // success — clear failure count
		processed++
	}

	if processed > 0 {
		logutil.FromCtx(ctx).Info("file processing worker processed files", "count", processed)
	}
}

func (w *ProcessingWorker) processOne(ctx context.Context, fileID string, doc *models.File) error {
	// Re-fetch to get the latest version (another write may have happened)
	latest, err := w.service.db.GetFileByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("process file %s: %w", fileID, err)
	}
	if latest == nil {
		// File was deleted between listing and processing — skip
		return nil
	}
	if latest.Processed {
		// Already processed (e.g. by a concurrent tick) — skip
		return nil
	}

	return w.service.ProcessFile(ctx, latest)
}
