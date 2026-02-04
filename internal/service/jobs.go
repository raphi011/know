// Package service provides business logic for Knowhow operations.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

// JobStatus represents the state of a background job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

// Job represents a background processing job.
type Job struct {
	ID          string
	Type        string // "ingest"
	Status      JobStatus
	Progress    int
	Total       int
	Result      *IngestResult
	Error       string
	StartedAt   time.Time
	CompletedAt *time.Time

	// Internal fields
	mu sync.RWMutex
}

// JobManager tracks and manages background jobs.
type JobManager struct {
	jobs        map[string]*Job
	mu          sync.RWMutex
	concurrency int
}

// NewJobManager creates a new job manager.
func NewJobManager(concurrency int) *JobManager {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &JobManager{
		jobs:        make(map[string]*Job),
		concurrency: concurrency,
	}
}

// Concurrency returns the configured concurrency level.
func (m *JobManager) Concurrency() int {
	return m.concurrency
}

// CreateJob creates a new pending job.
func (m *JobManager) CreateJob(jobType string) *Job {
	job := &Job{
		ID:        uuid.New().String()[:8], // Short ID for convenience
		Type:      jobType,
		Status:    JobStatusPending,
		StartedAt: time.Now(),
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	slog.Info("job created", "job_id", job.ID, "type", jobType)
	return job
}

// GetJob retrieves a job by ID.
func (m *JobManager) GetJob(id string) *Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs[id]
}

// ListJobs returns all jobs, most recent first.
func (m *JobManager) ListJobs() []*Job {
	m.mu.RLock()
	defer m.mu.RUnlock()

	jobs := make([]*Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		jobs = append(jobs, job)
	}

	// Sort by start time descending (most recent first)
	for i := 0; i < len(jobs)-1; i++ {
		for j := i + 1; j < len(jobs); j++ {
			if jobs[j].StartedAt.After(jobs[i].StartedAt) {
				jobs[i], jobs[j] = jobs[j], jobs[i]
			}
		}
	}

	return jobs
}

// UpdateProgress updates job progress.
func (j *Job) UpdateProgress(current, total int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Progress = current
	j.Total = total
	if j.Status == JobStatusPending {
		j.Status = JobStatusRunning
	}
}

// Complete marks job as completed with result.
func (j *Job) Complete(result *IngestResult) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = JobStatusCompleted
	j.Result = result
	now := time.Now()
	j.CompletedAt = &now
	slog.Info("job completed", "job_id", j.ID, "entities", result.EntitiesCreated, "errors", len(result.Errors))
}

// Fail marks job as failed with error.
func (j *Job) Fail(err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = JobStatusFailed
	j.Error = err.Error()
	now := time.Now()
	j.CompletedAt = &now
	slog.Error("job failed", "job_id", j.ID, "error", err)
}

// Snapshot returns a thread-safe copy of job state.
func (j *Job) Snapshot() Job {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return Job{
		ID:          j.ID,
		Type:        j.Type,
		Status:      j.Status,
		Progress:    j.Progress,
		Total:       j.Total,
		Result:      j.Result,
		Error:       j.Error,
		StartedAt:   j.StartedAt,
		CompletedAt: j.CompletedAt,
	}
}

// IngestDirectoryAsync starts an async ingestion job.
func (s *IngestService) IngestDirectoryAsync(ctx context.Context, jobManager *JobManager, dirPath string, opts IngestOptions) (*Job, error) {
	// Validate path exists before starting job
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path must be a directory: %s", dirPath)
	}

	job := jobManager.CreateJob("ingest")

	// Set concurrency from job manager and link job for progress tracking
	opts.Concurrency = jobManager.Concurrency()
	opts.Job = job

	// Start processing in background
	go func() {
		// Use background context since the HTTP request context will be cancelled
		bgCtx := context.Background()

		result, err := s.IngestDirectory(bgCtx, dirPath, opts)
		if err != nil {
			job.Fail(err)
			return
		}
		job.Complete(result)
	}()

	return job, nil
}
