package pipeline

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// mockJobStore is a test double for jobStore that records which method was called.
type mockJobStore struct {
	claimFn    func(ctx context.Context, limit int) ([]models.PipelineJob, error)
	completeFn func(ctx context.Context, jobID string) error
	retryFn    func(ctx context.Context, jobID string, runAfter time.Time) error
	failFn     func(ctx context.Context, jobID, errMsg string) error

	retryCalled atomic.Bool
	failCalled  atomic.Bool
}

func (m *mockJobStore) ClaimJobs(ctx context.Context, limit int) ([]models.PipelineJob, error) {
	if m.claimFn != nil {
		return m.claimFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockJobStore) CompleteJob(ctx context.Context, jobID string) error {
	if m.completeFn != nil {
		return m.completeFn(ctx, jobID)
	}
	return nil
}

func (m *mockJobStore) RetryJob(ctx context.Context, jobID string, runAfter time.Time) error {
	m.retryCalled.Store(true)
	if m.retryFn != nil {
		return m.retryFn(ctx, jobID, runAfter)
	}
	return nil
}

func (m *mockJobStore) FailJob(ctx context.Context, jobID, errMsg string) error {
	m.failCalled.Store(true)
	if m.failFn != nil {
		return m.failFn(ctx, jobID, errMsg)
	}
	return nil
}

func newTestWorker(store jobStore) *Worker {
	return &Worker{
		db:          store,
		handlers:    make(map[string]Handler),
		interval:    1 * time.Second,
		batch:       10,
		concurrency: 5,
	}
}

// TestWorker_StopsOnContextCancel verifies the worker exits cleanly when the context is cancelled.
func TestWorker_StopsOnContextCancel(t *testing.T) {
	store := &mockJobStore{}
	w := newTestWorker(store)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Worker stopped cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}

// TestWorker_Register verifies handler registration.
func TestWorker_Register(t *testing.T) {
	w := newTestWorker(&mockJobStore{})
	var called atomic.Bool
	w.Register("parse", func(_ context.Context, _ models.PipelineJob) error {
		called.Store(true)
		return nil
	})
	if _, ok := w.handlers["parse"]; !ok {
		t.Fatal("expected handler to be registered for 'parse'")
	}
}

// TestWorker_HandleFailure_RetryBelowMax verifies that RetryJob is called
// when attempt < max_attempts.
func TestWorker_HandleFailure_RetryBelowMax(t *testing.T) {
	store := &mockJobStore{}
	w := newTestWorker(store)

	job := models.PipelineJob{
		ID:          surrealmodels.RecordID{Table: "pipeline_job", ID: "test1"},
		Type:        "parse",
		Attempt:     0,
		MaxAttempts: 5,
	}

	w.handleFailure(context.Background(), job, "test1", errors.New("test error"))

	if !store.retryCalled.Load() {
		t.Fatal("expected RetryJob to be called (attempt 1 < max 5)")
	}
	if store.failCalled.Load() {
		t.Fatal("expected FailJob NOT to be called")
	}
}

// TestWorker_HandleFailure_FailAtMax verifies that FailJob is called
// when attempt+1 >= max_attempts.
func TestWorker_HandleFailure_FailAtMax(t *testing.T) {
	store := &mockJobStore{}
	w := newTestWorker(store)

	job := models.PipelineJob{
		ID:          surrealmodels.RecordID{Table: "pipeline_job", ID: "test2"},
		Type:        "parse",
		Attempt:     4,
		MaxAttempts: 5,
	}

	w.handleFailure(context.Background(), job, "test2", errors.New("test error"))

	if !store.failCalled.Load() {
		t.Fatal("expected FailJob to be called (attempt 5 >= max 5)")
	}
	if store.retryCalled.Load() {
		t.Fatal("expected RetryJob NOT to be called")
	}
}

// TestWorker_NewWorker_Panics verifies constructor panics on invalid arguments.
func TestWorker_NewWorker_Panics(t *testing.T) {
	tests := []struct {
		name        string
		interval    time.Duration
		batch       int
		concurrency int
	}{
		{"zero interval", 0, 1, 1},
		{"negative interval", -1, 1, 1},
		{"zero batch", 1 * time.Second, 0, 1},
		{"negative batch", 1 * time.Second, -1, 1},
		{"zero concurrency", 1 * time.Second, 1, 0},
		{"negative concurrency", 1 * time.Second, 1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic but did not panic")
				}
			}()
			NewWorker(nil, nil, tt.interval, tt.batch, tt.concurrency, nil)
		})
	}
}

// TestWorker_ConcurrentProcessing verifies that jobs are processed concurrently.
func TestWorker_ConcurrentProcessing(t *testing.T) {
	var running atomic.Int32
	var maxRunning atomic.Int32

	store := &mockJobStore{
		claimFn: func(_ context.Context, _ int) ([]models.PipelineJob, error) {
			return []models.PipelineJob{
				{ID: surrealmodels.RecordID{Table: "pipeline_job", ID: "j1"}, Type: "parse"},
				{ID: surrealmodels.RecordID{Table: "pipeline_job", ID: "j2"}, Type: "parse"},
				{ID: surrealmodels.RecordID{Table: "pipeline_job", ID: "j3"}, Type: "parse"},
			}, nil
		},
	}

	w := &Worker{
		db:          store,
		handlers:    make(map[string]Handler),
		interval:    1 * time.Second,
		batch:       10,
		concurrency: 3,
	}

	w.Register("parse", func(_ context.Context, _ models.PipelineJob) error {
		cur := running.Add(1)
		// Track max concurrent handlers running.
		for {
			old := maxRunning.Load()
			if cur <= old || maxRunning.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		running.Add(-1)
		return nil
	})

	w.tick(context.Background())

	if max := maxRunning.Load(); max < 2 {
		t.Errorf("expected concurrent execution (max running >= 2), got %d", max)
	}
}

// TestWorker_PanicRecovery verifies that a panicking handler does not crash the worker.
func TestWorker_PanicRecovery(t *testing.T) {
	var completed atomic.Int32

	store := &mockJobStore{
		claimFn: func(_ context.Context, _ int) ([]models.PipelineJob, error) {
			return []models.PipelineJob{
				{ID: surrealmodels.RecordID{Table: "pipeline_job", ID: "panic1"}, Type: "bad"},
				{ID: surrealmodels.RecordID{Table: "pipeline_job", ID: "ok1"}, Type: "good"},
			}, nil
		},
		completeFn: func(_ context.Context, _ string) error {
			completed.Add(1)
			return nil
		},
	}

	w := &Worker{
		db:          store,
		handlers:    make(map[string]Handler),
		interval:    1 * time.Second,
		batch:       10,
		concurrency: 2,
	}

	w.Register("bad", func(_ context.Context, _ models.PipelineJob) error {
		panic("test panic")
	})
	w.Register("good", func(_ context.Context, _ models.PipelineJob) error {
		return nil
	})

	// tick should not panic even though one handler panics.
	w.tick(context.Background())

	if c := completed.Load(); c < 1 {
		t.Errorf("expected at least 1 completed job (the good one), got %d", c)
	}
}
