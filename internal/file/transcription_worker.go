package file

import (
	"context"
	"time"

	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/logutil"
)

// TranscriptionWorker processes pending audio file transcriptions in the background.
// When an event bus is provided, wakes immediately on file.processed events.
type TranscriptionWorker struct {
	service  *Service
	batch    int
	interval time.Duration
	bus      *event.Bus
}

// NewTranscriptionWorker creates a worker that polls for pending audio transcriptions.
// If bus is non-nil, the worker also wakes immediately on file.processed events.
// Panics if service is nil, interval <= 0, or batchSize <= 0 (programmer errors).
func NewTranscriptionWorker(service *Service, interval time.Duration, batchSize int, bus *event.Bus) *TranscriptionWorker {
	if service == nil {
		panic("TranscriptionWorker: nil service")
	}
	if interval <= 0 {
		panic("TranscriptionWorker: interval must be positive")
	}
	if batchSize <= 0 {
		panic("TranscriptionWorker: batchSize must be positive")
	}
	return &TranscriptionWorker{
		service:  service,
		batch:    batchSize,
		interval: interval,
		bus:      bus,
	}
}

// Run starts the transcription worker loop. It blocks until the context is cancelled.
// Subscribes to bus events here (not in constructor) to avoid goroutine leaks
// if the worker is constructed but never started.
func (w *TranscriptionWorker) Run(ctx context.Context) {
	notify, unsub := eventNotify(w.bus, "file.processed")
	defer unsub()
	loop := NewWorkerLoop("transcription worker", w.interval, w.tick, notify)
	loop.Run(ctx)
}

func (w *TranscriptionWorker) tick(ctx context.Context) {
	n, err := w.service.TranscribePendingFiles(ctx, w.batch)
	if err != nil {
		logutil.FromCtx(ctx).Error("transcription worker error", "error", err)
		return
	}
	if n > 0 {
		logutil.FromCtx(ctx).Info("transcription worker processed files", "count", n)
	}
}
