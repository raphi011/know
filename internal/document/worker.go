package document

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
)

// EmbeddingWorker processes pending chunk embeddings in the background.
type EmbeddingWorker struct {
	service  *Service
	interval time.Duration
	batch    int
}

// NewEmbeddingWorker creates a worker that polls for pending chunk embeddings.
// Panics if service is nil, interval <= 0, or batchSize <= 0 (programmer errors).
func NewEmbeddingWorker(service *Service, interval time.Duration, batchSize int) *EmbeddingWorker {
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
		interval: interval,
		batch:    batchSize,
	}
}

// Run starts the embedding worker loop. It blocks until the context is cancelled.
// If the worker panics, it logs the stack trace and restarts after a short delay.
func (w *EmbeddingWorker) Run(ctx context.Context) {
	slog.Info("embedding worker started", "interval", w.interval, "batch_size", w.batch)

	for {
		stopped := w.runLoop(ctx)
		if stopped {
			return
		}
		// Panic recovery — wait before restarting to avoid a tight loop
		select {
		case <-ctx.Done():
			slog.Info("embedding worker stopped")
			return
		case <-time.After(5 * time.Second):
			slog.Info("embedding worker restarting after panic")
		}
	}
}

// runLoop runs the tick loop, returning true if the context was cancelled (clean stop)
// or false if a panic occurred and the worker should restart.
func (w *EmbeddingWorker) runLoop(ctx context.Context) (stopped bool) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("embedding worker panicked",
				"error", p, "stack", string(debug.Stack()))
			stopped = false
		}
	}()

	// Process any pending embeddings immediately on startup
	w.tick(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("embedding worker stopped")
			return true
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *EmbeddingWorker) tick(ctx context.Context) {
	n, err := w.service.EmbedPendingChunks(ctx, w.batch)
	if err != nil {
		slog.Error("embedding worker error", "error", err)
		return
	}
	if n > 0 {
		slog.Info("embedding worker processed chunks", "count", n)
	}
}
