package pipeline

import (
	"context"
	"time"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/metrics"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/worker"
)

// Handler is a function that processes a single pipeline job.
type Handler func(ctx context.Context, job models.PipelineJob) error

// jobStore is the subset of db.Client methods used by Worker.
type jobStore interface {
	ClaimJobs(ctx context.Context, limit int) ([]models.PipelineJob, error)
	CompleteJob(ctx context.Context, jobID string) error
	RetryJob(ctx context.Context, jobID string, runAfter time.Time) error
	FailJob(ctx context.Context, jobID, errMsg string) error
}

// Worker claims jobs from the pipeline_job table and dispatches them to
// registered handlers by job type. A single Worker goroutine replaces the
// three separate processing/embedding/transcription workers.
type Worker struct {
	db       jobStore
	handlers map[string]Handler
	bus      *event.Bus
	metrics  *metrics.Metrics
	interval time.Duration
	batch    int
}

// NewWorker creates a new PipelineWorker.
// Panics if interval <= 0 or batch <= 0 (programmer errors).
func NewWorker(dbClient *db.Client, bus *event.Bus, interval time.Duration, batch int, m *metrics.Metrics) *Worker {
	if interval <= 0 {
		panic("pipeline.Worker: interval must be positive")
	}
	if batch <= 0 {
		panic("pipeline.Worker: batch must be positive")
	}
	return &Worker{
		db:       dbClient,
		handlers: make(map[string]Handler),
		bus:      bus,
		metrics:  m,
		interval: interval,
		batch:    batch,
	}
}

// Register associates a handler with a job type.
// Must be called before Run.
func (w *Worker) Register(jobType string, handler Handler) {
	w.handlers[jobType] = handler
}

// Run starts the worker loop. It blocks until ctx is cancelled.
// Subscribes to bus events here (not in constructor) to avoid goroutine leaks.
func (w *Worker) Run(ctx context.Context) {
	notify, unsub := worker.EventNotify(w.bus, "job.created")
	defer unsub()
	loop := worker.NewWorkerLoop("pipeline worker", w.interval, w.tick, notify)
	loop.Run(ctx)
}

func (w *Worker) tick(ctx context.Context) {
	jobs, err := w.db.ClaimJobs(ctx, w.batch)
	if err != nil {
		logutil.FromCtx(ctx).Error("pipeline worker: claim jobs", "error", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	logger := logutil.FromCtx(ctx)
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}

		jobID, err := models.RecordIDString(job.ID)
		if err != nil {
			logger.Warn("pipeline worker: failed to extract job ID", "error", err)
			continue
		}

		handler, ok := w.handlers[job.Type]
		if !ok {
			logger.Warn("pipeline worker: no handler for job type, skipping", "type", job.Type, "job_id", jobID)
			if err := w.db.CompleteJob(ctx, jobID); err != nil {
				logger.Error("pipeline worker: complete unhandled job", "job_id", jobID, "error", err)
			}
			continue
		}

		start := time.Now()
		if err := handler(ctx, job); err != nil {
			w.handleFailure(ctx, job, jobID, err)
			if w.metrics != nil {
				w.metrics.RecordPipelineJob(job.Type, "failed", time.Since(start))
			}
			continue
		}

		if err := w.db.CompleteJob(ctx, jobID); err != nil {
			logger.Error("pipeline worker: complete job", "job_id", jobID, "type", job.Type, "error", err)
		}
		if w.metrics != nil {
			w.metrics.RecordPipelineJob(job.Type, "completed", time.Since(start))
		}
	}
}

func (w *Worker) handleFailure(ctx context.Context, job models.PipelineJob, jobID string, err error) {
	logger := logutil.FromCtx(ctx)
	nextAttempt := job.Attempt + 1
	if nextAttempt < job.MaxAttempts {
		// TODO: use exponential backoff (e.g. 30s, 60s, 120s) for transient API errors.
		runAfter := time.Now().Add(30 * time.Second)
		if retryErr := w.db.RetryJob(ctx, jobID, runAfter); retryErr != nil {
			logger.Error("pipeline worker: retry job", "job_id", jobID, "type", job.Type, "error", retryErr)
			return
		}
		logger.Warn("pipeline worker: job failed, will retry",
			"job_id", jobID, "type", job.Type,
			"attempt", nextAttempt, "max_attempts", job.MaxAttempts, "error", err)
	} else {
		if failErr := w.db.FailJob(ctx, jobID, err.Error()); failErr != nil {
			logger.Error("pipeline worker: fail job", "job_id", jobID, "type", job.Type, "error", failErr)
			return
		}
		logger.Error("pipeline worker: job permanently failed",
			"job_id", jobID, "type", job.Type,
			"attempts", nextAttempt, "error", err)
	}
}
