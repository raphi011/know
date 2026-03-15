package document

import (
	"context"
	"time"

	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/logutil"
)

// EmbeddingWorker processes pending chunk embeddings in the background.
// When an event bus is provided, wakes immediately on document.processed events.
type EmbeddingWorker struct {
	service  *Service
	batch    int
	interval time.Duration
	bus      *event.Bus
}

// NewEmbeddingWorker creates a worker that polls for pending chunk embeddings.
// If bus is non-nil, the worker also wakes immediately on document.processed events.
// Panics if service is nil, interval <= 0, or batchSize <= 0 (programmer errors).
func NewEmbeddingWorker(service *Service, interval time.Duration, batchSize int, bus *event.Bus) *EmbeddingWorker {
	if service == nil {
		panic("EmbeddingWorker: nil service")
	}
	if interval <= 0 {
		panic("EmbeddingWorker: interval must be positive")
	}
	if batchSize <= 0 {
		panic("EmbeddingWorker: batchSize must be positive")
	}

	return &EmbeddingWorker{
		service:  service,
		batch:    batchSize,
		interval: interval,
		bus:      bus,
	}
}

// Run starts the embedding worker loop. It blocks until the context is cancelled.
// Subscribes to bus events here (not in constructor) to avoid goroutine leaks
// if the worker is constructed but never started.
func (w *EmbeddingWorker) Run(ctx context.Context) {
	notify, unsub := eventNotify(w.bus, "document.processed")
	defer unsub()
	loop := NewWorkerLoop("embedding worker", w.interval, w.tick, notify)
	loop.Run(ctx)
}

func (w *EmbeddingWorker) tick(ctx context.Context) {
	n, err := w.service.EmbedPendingChunks(ctx, w.batch)
	if err != nil {
		logutil.FromCtx(ctx).Error("embedding worker error", "error", err)
		return
	}
	if n > 0 {
		logutil.FromCtx(ctx).Info("embedding worker processed chunks", "count", n)
	}
}
