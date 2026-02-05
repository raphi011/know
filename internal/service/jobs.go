// Package service provides business logic for Knowhow operations.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/models"
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
	Name        string   // User-provided name for rerunning
	Labels      []string // Curated labels applied to entities
	Progress    int
	Total       int
	Result      *IngestResult
	Error       string
	StartedAt   time.Time
	CompletedAt *time.Time

	// Persistence fields
	DirPath      string   // Directory being ingested
	Files        []string // All files to process
	PendingFiles int      // Files remaining (for resume)

	// Internal fields
	mu                 sync.RWMutex
	lastProgressUpdate time.Time // For debouncing DB writes
}

// JobManager tracks and manages background jobs.
type JobManager struct {
	jobs        map[string]*Job
	mu          sync.RWMutex
	concurrency int
	db          *db.Client
}

// NewJobManager creates a new job manager.
func NewJobManager(concurrency int, dbClient *db.Client) *JobManager {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &JobManager{
		jobs:        make(map[string]*Job),
		concurrency: concurrency,
		db:          dbClient,
	}
}

// Concurrency returns the configured concurrency level.
func (m *JobManager) Concurrency() int {
	return m.concurrency
}

// CreateJob creates a new pending job with persistence.
func (m *JobManager) CreateJob(ctx context.Context, jobType, name, dirPath string, files, labels []string, opts map[string]any) (*Job, error) {
	job := &Job{
		ID:        uuid.New().String()[:8], // Short ID for convenience
		Type:      jobType,
		Status:    JobStatusPending,
		Name:      name,
		Labels:    labels,
		StartedAt: time.Now(),
		DirPath:   dirPath,
		Files:     files,
		Total:     len(files),
	}

	// Persist to database
	if m.db != nil {
		if err := m.db.CreateIngestJob(ctx, job.ID, name, dirPath, files, labels, opts); err != nil {
			return nil, err
		}
	}

	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	slog.Info("job created", "job_id", job.ID, "name", name, "type", jobType, "files", len(files))
	return job, nil
}

// RegisterJob adds an existing job to the in-memory map (for resume).
func (m *JobManager) RegisterJob(job *Job) {
	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()
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
	slices.SortFunc(jobs, func(a, b *Job) int {
		return b.StartedAt.Compare(a.StartedAt)
	})

	return jobs
}

// UpdateProgress updates job progress with debounced DB persistence.
func (m *JobManager) UpdateProgress(ctx context.Context, job *Job, current, total int) {
	job.mu.Lock()
	job.Progress = current
	job.Total = total
	if job.Status == JobStatusPending {
		job.Status = JobStatusRunning
	}

	// Debounce DB updates - only persist every 5 seconds or every 10 files
	shouldPersist := m.db != nil && (time.Since(job.lastProgressUpdate) > 5*time.Second ||
		current%10 == 0 || current == total)
	if shouldPersist {
		job.lastProgressUpdate = time.Now()
	}
	job.mu.Unlock()

	if shouldPersist {
		if err := m.db.UpdateJobProgress(ctx, job.ID, current); err != nil {
			slog.Warn("failed to persist job progress", "job_id", job.ID, "error", err)
		}
	}
}

// SetRunning marks job as running in DB.
func (m *JobManager) SetRunning(ctx context.Context, job *Job) {
	job.mu.Lock()
	job.Status = JobStatusRunning
	job.mu.Unlock()

	if m.db != nil {
		if err := m.db.UpdateJobStatus(ctx, job.ID, string(JobStatusRunning)); err != nil {
			slog.Warn("failed to set job running", "job_id", job.ID, "error", err)
		}
	}
}

// Complete marks job as completed with result.
func (m *JobManager) Complete(ctx context.Context, job *Job, result *IngestResult) {
	job.mu.Lock()
	job.Status = JobStatusCompleted
	job.Result = result
	now := time.Now()
	job.CompletedAt = &now
	job.mu.Unlock()

	if m.db != nil {
		resultMap := map[string]any{
			"files_processed":   result.FilesProcessed,
			"entities_created":  result.EntitiesCreated,
			"chunks_created":    result.ChunksCreated,
			"relations_created": result.RelationsCreated,
			"errors":            result.Errors,
		}
		if err := m.db.CompleteJob(ctx, job.ID, resultMap); err != nil {
			slog.Warn("failed to persist job completion", "job_id", job.ID, "error", err)
		}
	}

	slog.Info("job completed", "job_id", job.ID, "entities", result.EntitiesCreated, "errors", len(result.Errors))
}

// Fail marks job as failed with error.
func (m *JobManager) Fail(ctx context.Context, job *Job, err error) {
	job.mu.Lock()
	job.Status = JobStatusFailed
	job.Error = err.Error()
	now := time.Now()
	job.CompletedAt = &now
	job.mu.Unlock()

	if m.db != nil {
		if dbErr := m.db.FailJob(ctx, job.ID, err.Error()); dbErr != nil {
			slog.Warn("failed to persist job failure", "job_id", job.ID, "error", dbErr)
		}
	}

	slog.Error("job failed", "job_id", job.ID, "error", err)
}

// ResumeIncompleteJobs resumes any incomplete jobs from the database.
func (m *JobManager) ResumeIncompleteJobs(ctx context.Context, ingestService *IngestService) error {
	if m.db == nil {
		return nil
	}

	incompleteJobs, err := m.db.GetIncompleteJobs(ctx)
	if err != nil {
		return err
	}

	if len(incompleteJobs) == 0 {
		slog.Info("no incomplete jobs to resume")
		return nil
	}

	slog.Info("found incomplete jobs", "count", len(incompleteJobs))

	for _, dbJob := range incompleteJobs {
		jobID, err := models.RecordIDString(dbJob.ID)
		if err != nil {
			slog.Warn("failed to get job ID", "error", err)
			continue
		}

		// Skip content-based jobs - they can't be resumed because file content
		// was provided by the client, not read from disk. User should re-run CLI.
		if dbJob.Options != nil {
			if contentBased, ok := dbJob.Options["content_based"].(bool); ok && contentBased {
				slog.Info("skipping content-based job (requires client re-trigger)", "job_id", jobID)
				continue
			}
		}

		// Check which files have already been processed
		existingPaths, err := m.db.GetEntitiesBySourcePaths(ctx, dbJob.Files)
		if err != nil {
			slog.Warn("failed to check existing entities", "job_id", jobID, "error", err)
			continue
		}

		// Build set of processed paths
		processedSet := make(map[string]bool, len(existingPaths))
		for _, p := range existingPaths {
			processedSet[p] = true
		}

		// Filter to pending files
		pendingFiles := make([]string, 0, len(dbJob.Files)-len(existingPaths))
		for _, f := range dbJob.Files {
			if !processedSet[f] {
				pendingFiles = append(pendingFiles, f)
			}
		}

		slog.Info("resuming job",
			"job_id", jobID,
			"total_files", len(dbJob.Files),
			"processed", len(existingPaths),
			"pending", len(pendingFiles))

		// If all files processed, mark as completed
		if len(pendingFiles) == 0 {
			slog.Info("job already complete, marking as completed", "job_id", jobID)
			if err := m.db.CompleteJob(ctx, jobID, map[string]any{
				"files_processed":  len(dbJob.Files),
				"entities_created": len(existingPaths),
				"resumed":          true,
			}); err != nil {
				slog.Warn("failed to mark resumed job complete", "job_id", jobID, "error", err)
			}
			continue
		}

		// Extract name from dbJob
		var name string
		if dbJob.Name != nil {
			name = *dbJob.Name
		}

		// Create in-memory job
		job := &Job{
			ID:           jobID,
			Type:         dbJob.JobType,
			Status:       JobStatusRunning,
			Name:         name,
			Labels:       dbJob.Labels,
			Progress:     len(existingPaths),
			Total:        len(dbJob.Files),
			StartedAt:    dbJob.StartedAt,
			DirPath:      dbJob.DirPath,
			Files:        pendingFiles,
			PendingFiles: len(pendingFiles),
		}

		m.RegisterJob(job)

		// Resume processing in background
		go func(job *Job, pendingFiles []string, dbJob models.IngestJob) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("resumed job goroutine panicked", "job_id", job.ID, "panic", r)
					m.Fail(context.Background(), job, fmt.Errorf("internal panic: %v", r))
				}
			}()

			bgCtx := context.Background()

			// Parse options from stored job
			opts := IngestOptions{
				Concurrency: m.concurrency,
			}
			if dbJob.Options != nil {
				if labels, ok := dbJob.Options["labels"].([]any); ok {
					for _, l := range labels {
						if s, ok := l.(string); ok {
							opts.Labels = append(opts.Labels, s)
						}
					}
				}
				if extractGraph, ok := dbJob.Options["extract_graph"].(bool); ok {
					opts.ExtractGraph = extractGraph
				}
				if recursive, ok := dbJob.Options["recursive"].(bool); ok {
					opts.Recursive = recursive
				}
			}

			result, err := ingestService.ProcessFiles(bgCtx, m, job, pendingFiles, opts)
			if err != nil {
				m.Fail(bgCtx, job, err)
				return
			}
			m.Complete(bgCtx, job, result)
		}(job, pendingFiles, dbJob)
	}

	return nil
}

// Snapshot returns a thread-safe copy of job state.
func (j *Job) Snapshot() Job {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return Job{
		ID:           j.ID,
		Type:         j.Type,
		Status:       j.Status,
		Name:         j.Name,
		Labels:       j.Labels,
		Progress:     j.Progress,
		Total:        j.Total,
		Result:       j.Result,
		Error:        j.Error,
		StartedAt:    j.StartedAt,
		CompletedAt:  j.CompletedAt,
		DirPath:      j.DirPath,
		PendingFiles: j.PendingFiles,
	}
}
