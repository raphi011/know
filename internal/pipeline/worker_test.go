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
		db:       store,
		handlers: make(map[string]Handler),
		interval: 1 * time.Second,
		batch:    10,
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
		name     string
		interval time.Duration
		batch    int
	}{
		{"zero interval", 0, 1},
		{"negative interval", -1, 1},
		{"zero batch", 1 * time.Second, 0},
		{"negative batch", 1 * time.Second, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic but did not panic")
				}
			}()
			NewWorker(nil, nil, tt.interval, tt.batch)
		})
	}
}
