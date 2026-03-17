package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// CreateJob inserts a new pending pipeline job for the given file.
func (c *Client) CreateJob(ctx context.Context, fileID, jobType string, priority int) error {
	defer c.logOp(ctx, "pipeline_job.create", time.Now())
	sql := `INSERT INTO pipeline_job {
		file:     type::record("file", $file_id),
		type:     $type,
		status:   "pending",
		priority: $priority
	}`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"file_id":  bareID("file", fileID),
		"type":     jobType,
		"priority": priority,
	}); err != nil {
		return fmt.Errorf("create job: %w", err)
	}
	return nil
}

// ClaimJobs atomically claims up to limit pending jobs by setting status='running'.
// Returns the job state before the claim (BEFORE), so callers get the original data.
func (c *Client) ClaimJobs(ctx context.Context, limit int) ([]models.PipelineJob, error) {
	defer c.logOp(ctx, "pipeline_job.claim", time.Now())
	sql := `
		UPDATE (
			SELECT * FROM pipeline_job
			WHERE status = 'pending'
			  AND (run_after IS NONE OR run_after <= time::now())
			ORDER BY priority DESC, created_at ASC
			LIMIT $limit
		)
		SET status = 'running'
		RETURN BEFORE
	`
	results, err := surrealdb.Query[[]models.PipelineJob](ctx, c.DB(), sql, map[string]any{
		"limit": limit,
	})
	if err != nil {
		return nil, fmt.Errorf("claim jobs: %w", err)
	}
	return allResults(results), nil
}

// CompleteJob marks a job as done.
func (c *Client) CompleteJob(ctx context.Context, jobID string) error {
	defer c.logOp(ctx, "pipeline_job.complete", time.Now())
	sql := `UPDATE type::record("pipeline_job", $id) SET status = 'done'`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id": bareID("pipeline_job", jobID),
	}); err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

// FailJob marks a job as permanently failed with an error message.
func (c *Client) FailJob(ctx context.Context, jobID, errMsg string) error {
	defer c.logOp(ctx, "pipeline_job.fail", time.Now())
	sql := `UPDATE type::record("pipeline_job", $id) SET status = 'failed', error = $error`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id":    bareID("pipeline_job", jobID),
		"error": errMsg,
	}); err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	return nil
}

// RetryJob resets a job to pending with an incremented attempt count and a future run_after.
func (c *Client) RetryJob(ctx context.Context, jobID string, runAfter time.Time) error {
	defer c.logOp(ctx, "pipeline_job.retry", time.Now())
	sql := `UPDATE type::record("pipeline_job", $id) SET status = 'pending', attempt = attempt + 1, run_after = $run_after`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id":        bareID("pipeline_job", jobID),
		"run_after": runAfter,
	}); err != nil {
		return fmt.Errorf("retry job: %w", err)
	}
	return nil
}

// CancelJobsForFile cancels all pending/running jobs for a file (e.g. when the file is re-ingested).
// Sets status='cancelled' to distinguish superseded jobs from successfully completed ones.
func (c *Client) CancelJobsForFile(ctx context.Context, fileID string) error {
	defer c.logOp(ctx, "pipeline_job.cancel_for_file", time.Now())
	sql := `UPDATE pipeline_job SET status = 'cancelled' WHERE file = type::record("file", $file_id) AND status IN ['pending', 'running']`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"file_id": bareID("file", fileID),
	}); err != nil {
		return fmt.Errorf("cancel jobs for file: %w", err)
	}
	return nil
}

// ReconcileStaleRunningJobs resets any jobs stuck in "running" back to "pending"
// so they are retried on the next tick. Called on startup to recover from an
// unclean shutdown where the worker was killed mid-job.
func (c *Client) ReconcileStaleRunningJobs(ctx context.Context) (int, error) {
	defer c.logOp(ctx, "pipeline_job.reconcile_stale", time.Now())
	sql := `UPDATE pipeline_job SET status = 'pending' WHERE status = 'running' RETURN AFTER`
	results, err := surrealdb.Query[[]models.PipelineJob](ctx, c.DB(), sql, nil)
	if err != nil {
		return 0, fmt.Errorf("reconcile stale running jobs: %w", err)
	}
	return countResults(results), nil
}
