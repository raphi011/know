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
		SET status = 'running', started_at = time::now()
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
	sql := `UPDATE type::record("pipeline_job", $id) SET status = 'done', completed_at = time::now()`
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
	sql := `UPDATE type::record("pipeline_job", $id) SET status = 'failed', error = $error, completed_at = time::now()`
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
	sql := `UPDATE type::record("pipeline_job", $id) SET status = 'pending', attempt = attempt + 1, run_after = $run_after, started_at = NONE, completed_at = NONE`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id":        bareID("pipeline_job", jobID),
		"run_after": runAfter,
	}); err != nil {
		return fmt.Errorf("retry job: %w", err)
	}
	return nil
}

// GetJobStats returns aggregate job counts by status for jobs created since the given time.
func (c *Client) GetJobStats(ctx context.Context, since time.Time) (*models.JobStats, error) {
	defer c.logOp(ctx, "pipeline_job.stats", time.Now())
	sql := `SELECT
		math::sum(IF status = 'pending' THEN 1 ELSE 0 END) AS pending,
		math::sum(IF status = 'running' THEN 1 ELSE 0 END) AS running,
		math::sum(IF status = 'done' THEN 1 ELSE 0 END) AS done,
		math::sum(IF status = 'failed' THEN 1 ELSE 0 END) AS failed,
		math::sum(IF status = 'cancelled' THEN 1 ELSE 0 END) AS cancelled
	FROM pipeline_job WHERE created_at >= $since GROUP ALL`
	results, err := surrealdb.Query[[]models.JobStats](ctx, c.DB(), sql, map[string]any{
		"since": since,
	})
	if err != nil {
		return nil, fmt.Errorf("get job stats: %w", err)
	}
	rows := allResults(results)
	if len(rows) == 0 {
		return &models.JobStats{}, nil
	}
	return &rows[0], nil
}

// ListRecentJobs returns recent jobs filtered by status, with the related file path.
func (c *Client) ListRecentJobs(ctx context.Context, limit int, statuses []string) ([]models.PipelineJobDetail, error) {
	defer c.logOp(ctx, "pipeline_job.list_recent", time.Now())
	sql := `SELECT *, file.path AS file_path
		FROM pipeline_job
		WHERE status IN $statuses
		ORDER BY created_at DESC
		LIMIT $limit`
	results, err := surrealdb.Query[[]models.PipelineJobDetail](ctx, c.DB(), sql, map[string]any{
		"statuses": statuses,
		"limit":    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list recent jobs: %w", err)
	}
	return allResults(results), nil
}

// GetJobTypeDurations returns per-type duration stats (min/max/avg) for completed jobs since the given time.
func (c *Client) GetJobTypeDurations(ctx context.Context, since time.Time) ([]models.JobTypeDuration, error) {
	defer c.logOp(ctx, "pipeline_job.type_durations", time.Now())
	sql := `SELECT type,
		count() AS count,
		math::min(duration::millis(completed_at - started_at)) AS min_ms,
		math::max(duration::millis(completed_at - started_at)) AS max_ms,
		math::mean(duration::millis(completed_at - started_at)) AS avg_ms
	FROM pipeline_job
	WHERE status = 'done' AND completed_at >= $since AND started_at IS NOT NONE
	GROUP BY type`
	results, err := surrealdb.Query[[]models.JobTypeDuration](ctx, c.DB(), sql, map[string]any{
		"since": since,
	})
	if err != nil {
		return nil, fmt.Errorf("get job type durations: %w", err)
	}
	return allResults(results), nil
}

// CancelJobsForFile cancels all pending/running jobs for a file (e.g. when the file is re-ingested).
// Sets status='cancelled' to distinguish superseded jobs from successfully completed ones.
func (c *Client) CancelJobsForFile(ctx context.Context, fileID string) error {
	defer c.logOp(ctx, "pipeline_job.cancel_for_file", time.Now())
	sql := `UPDATE pipeline_job SET status = 'cancelled', completed_at = time::now() WHERE file = type::record("file", $file_id) AND status IN ['pending', 'running']`
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
	sql := `UPDATE pipeline_job SET status = 'pending', started_at = NONE, completed_at = NONE WHERE status = 'running' RETURN AFTER`
	results, err := surrealdb.Query[[]models.PipelineJob](ctx, c.DB(), sql, nil)
	if err != nil {
		return 0, fmt.Errorf("reconcile stale running jobs: %w", err)
	}
	return countResults(results), nil
}

// CancelPendingJobs cancels all pending/running jobs, optionally filtered by vault.
// Returns the number of cancelled jobs.
func (c *Client) CancelPendingJobs(ctx context.Context, vaultID string) (int, error) {
	defer c.logOp(ctx, "pipeline_job.cancel_pending", time.Now())
	var sql string
	vars := map[string]any{}

	if vaultID != "" {
		sql = `UPDATE pipeline_job SET status = 'cancelled', completed_at = time::now()
			WHERE status IN ['pending', 'running']
			  AND file.vault = type::record("vault", $vault_id)
			RETURN AFTER`
		vars["vault_id"] = bareID("vault", vaultID)
	} else {
		sql = `UPDATE pipeline_job SET status = 'cancelled', completed_at = time::now()
			WHERE status IN ['pending', 'running']
			RETURN AFTER`
	}

	results, err := surrealdb.Query[[]models.PipelineJob](ctx, c.DB(), sql, vars)
	if err != nil {
		return 0, fmt.Errorf("cancel pending jobs: %w", err)
	}
	return countResults(results), nil
}

// EnqueueReprocessJobs creates parse/pdf/transcribe jobs for all files,
// optionally filtered by vault. Returns the number of jobs created.
func (c *Client) EnqueueReprocessJobs(ctx context.Context, vaultID string) (int, error) {
	defer c.logOp(ctx, "pipeline_job.enqueue_reprocess", time.Now())

	vaultFilter := ""
	vars := map[string]any{}
	if vaultID != "" {
		vaultFilter = ` AND vault = type::record("vault", $vault_id)`
		vars["vault_id"] = bareID("vault", vaultID)
	}

	// Insert jobs in three batches by file type to avoid nested IF syntax issues.
	// SAFETY: jobType and mimeWhere are hardcoded literals, never from user input.
	total := 0
	queries := []struct {
		jobType   string
		mimeWhere string
	}{
		{"transcribe", `string::starts_with(mime_type, "audio/")`},
		{"pdf", `mime_type = "application/pdf"`},
		{"parse", `!string::starts_with(mime_type, "audio/") AND mime_type != "application/pdf"`},
	}

	for _, q := range queries {
		sql := fmt.Sprintf(`INSERT INTO pipeline_job (
			SELECT id AS file, "%s" AS type, "pending" AS status, 0 AS priority
			FROM file WHERE is_folder = false AND %s%s
		)`, q.jobType, q.mimeWhere, vaultFilter)

		results, err := surrealdb.Query[[]models.PipelineJob](ctx, c.DB(), sql, vars)
		if err != nil {
			return total, fmt.Errorf("enqueue reprocess %s jobs: %w", q.jobType, err)
		}
		total += countResults(results)
	}

	return total, nil
}
