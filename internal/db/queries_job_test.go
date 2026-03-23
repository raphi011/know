package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestCreateJob(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-create-" + suffix + ".md", Title: "Job Create",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateJob(ctx, fileID, "embed", 10); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	jobs, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Type != "embed" {
		t.Errorf("expected type 'embed', got %q", jobs[0].Type)
	}
	if jobs[0].Priority != 10 {
		t.Errorf("expected priority 10, got %d", jobs[0].Priority)
	}
	// RETURN BEFORE: status in the returned record is the pre-update value
	if jobs[0].Status != "pending" {
		t.Errorf("expected status 'pending' (BEFORE), got %q", jobs[0].Status)
	}
}

func TestClaimJobs_PriorityOrder(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	fileA, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-prio-a-" + suffix + ".md", Title: "Job Prio A",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile A failed: %v", err)
	}
	fileB, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-prio-b-" + suffix + ".md", Title: "Job Prio B",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile B failed: %v", err)
	}
	fileAID := models.MustRecordIDString(fileA.ID)
	fileBID := models.MustRecordIDString(fileB.ID)

	// Low priority first, then high — claim should return high priority first
	if err := testDB.CreateJob(ctx, fileAID, "embed", 1); err != nil {
		t.Fatalf("CreateJob low priority failed: %v", err)
	}
	if err := testDB.CreateJob(ctx, fileBID, "embed", 100); err != nil {
		t.Fatalf("CreateJob high priority failed: %v", err)
	}

	// Claim only 1 job — should be the high-priority one
	jobs, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(jobs))
	}
	if jobs[0].Priority != 100 {
		t.Errorf("expected high-priority job (100) to be claimed first, got priority %d", jobs[0].Priority)
	}
}

func TestClaimJobs_RespectsRunAfter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-runafter-" + suffix + ".md", Title: "Job RunAfter",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	// Create job, then retry it with a far-future run_after
	if err := testDB.CreateJob(ctx, fileID, "embed", 5); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Claim once to move it to running
	claimed, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs (first) failed: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 job to claim initially, got %d", len(claimed))
	}
	jobID := models.MustRecordIDString(claimed[0].ID)

	// Retry with future run_after
	future := time.Now().Add(24 * time.Hour)
	if err := testDB.RetryJob(ctx, jobID, future); err != nil {
		t.Fatalf("RetryJob failed: %v", err)
	}

	// Should not be claimable (run_after is in the future)
	jobs, err := testDB.ClaimJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimJobs (second) failed: %v", err)
	}
	for _, j := range jobs {
		if models.MustRecordIDString(j.ID) == jobID {
			t.Error("job with future run_after should not be claimable")
		}
	}
}

func TestClaimJobs_RespectsLimit(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	for i := range 3 {
		file, err := testDB.CreateFile(ctx, models.FileInput{
			VaultID: vaultID,
			Path:    fmt.Sprintf("/job-limit-%s-%d.md", suffix, i),
			Title:   fmt.Sprintf("Job Limit %d", i),
			Content: "content", Labels: []string{},
		})
		if err != nil {
			t.Fatalf("CreateFile %d failed: %v", i, err)
		}
		if err := testDB.CreateJob(ctx, models.MustRecordIDString(file.ID), "embed", 1); err != nil {
			t.Fatalf("CreateJob %d failed: %v", i, err)
		}
	}

	jobs, err := testDB.ClaimJobs(ctx, 2)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected exactly 2 jobs claimed (limit=2), got %d", len(jobs))
	}
}

func TestCompleteJob(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-complete-" + suffix + ".md", Title: "Job Complete",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateJob(ctx, fileID, "embed", 1); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	claimed, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}
	jobID := models.MustRecordIDString(claimed[0].ID)

	if err := testDB.CompleteJob(ctx, jobID); err != nil {
		t.Fatalf("CompleteJob failed: %v", err)
	}

	// Completed job should not be returned by ClaimJobs
	jobs, err := testDB.ClaimJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimJobs after complete failed: %v", err)
	}
	for _, j := range jobs {
		if models.MustRecordIDString(j.ID) == jobID {
			t.Error("completed job should not be claimable again")
		}
	}

	// Verify started_at and completed_at are set via ListRecentJobs
	doneJobs, err := testDB.ListRecentJobs(ctx, 10, []string{"done"})
	if err != nil {
		t.Fatalf("ListRecentJobs failed: %v", err)
	}
	for _, j := range doneJobs {
		if models.MustRecordIDString(j.ID) == jobID {
			if j.StartedAt == nil {
				t.Error("expected started_at to be set on completed job")
			}
			if j.CompletedAt == nil {
				t.Error("expected completed_at to be set on completed job")
			}
			if j.StartedAt != nil && j.CompletedAt != nil && j.CompletedAt.Before(*j.StartedAt) {
				t.Error("expected completed_at >= started_at")
			}
			break
		}
	}
}

func TestFailJob(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-fail-" + suffix + ".md", Title: "Job Fail",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateJob(ctx, fileID, "embed", 1); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	claimed, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}
	jobID := models.MustRecordIDString(claimed[0].ID)

	errMsg := "embedding model unavailable"
	if err := testDB.FailJob(ctx, jobID, errMsg); err != nil {
		t.Fatalf("FailJob failed: %v", err)
	}

	// Failed job must not be claimable
	jobs, err := testDB.ClaimJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimJobs after fail failed: %v", err)
	}
	for _, j := range jobs {
		if models.MustRecordIDString(j.ID) == jobID {
			t.Error("failed job should not be claimable")
		}
	}

	// Verify completed_at is set on failed job
	failedJobs, err := testDB.ListRecentJobs(ctx, 10, []string{"failed"})
	if err != nil {
		t.Fatalf("ListRecentJobs failed: %v", err)
	}
	for _, j := range failedJobs {
		if models.MustRecordIDString(j.ID) == jobID {
			if j.CompletedAt == nil {
				t.Error("expected completed_at to be set on failed job")
			}
			break
		}
	}
}

func TestRetryJob(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-retry-" + suffix + ".md", Title: "Job Retry",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateJob(ctx, fileID, "embed", 1); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Claim (attempt=0, status=pending before claim → running after)
	claimed, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed job, got %d", len(claimed))
	}
	jobID := models.MustRecordIDString(claimed[0].ID)

	// Retry with a past run_after so the job becomes claimable immediately
	past := time.Now().Add(-1 * time.Second)
	if err := testDB.RetryJob(ctx, jobID, past); err != nil {
		t.Fatalf("RetryJob failed: %v", err)
	}

	// Should be claimable again (status reset to pending)
	jobs, err := testDB.ClaimJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimJobs after retry failed: %v", err)
	}
	var found *models.PipelineJob
	for i := range jobs {
		if models.MustRecordIDString(jobs[i].ID) == jobID {
			found = &jobs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("retried job should be claimable again")
	}
	// RETURN BEFORE gives us the pre-update value; status should be "pending"
	// and attempt should now be 1 (set by RetryJob before this claim)
	if found.Attempt != 1 {
		t.Errorf("expected attempt=1 after retry, got %d", found.Attempt)
	}
}

func TestCancelJobsForFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-cancel-" + suffix + ".md", Title: "Job Cancel",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	// Create 2 pending jobs for the file
	if err := testDB.CreateJob(ctx, fileID, "embed", 1); err != nil {
		t.Fatalf("CreateJob 1 failed: %v", err)
	}
	if err := testDB.CreateJob(ctx, fileID, "chunk", 1); err != nil {
		t.Fatalf("CreateJob 2 failed: %v", err)
	}

	// Create a third job and complete it — completed jobs must not be cancelled
	if err := testDB.CreateJob(ctx, fileID, "index", 1); err != nil {
		t.Fatalf("CreateJob 3 failed: %v", err)
	}
	doneJobs, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs for done job failed: %v", err)
	}
	if len(doneJobs) > 0 {
		if err := testDB.CompleteJob(ctx, models.MustRecordIDString(doneJobs[0].ID)); err != nil {
			t.Fatalf("CompleteJob failed: %v", err)
		}
	}

	if err := testDB.CancelJobsForFile(ctx, fileID); err != nil {
		t.Fatalf("CancelJobsForFile failed: %v", err)
	}

	// After cancellation the 2 pending jobs must not be claimable
	jobs, err := testDB.ClaimJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimJobs after cancel failed: %v", err)
	}
	for _, j := range jobs {
		if models.MustRecordIDString(j.File) == fileID {
			t.Errorf("cancelled job for file should not be claimable (job status: %s)", j.Status)
		}
	}
}

func TestGetJobStats(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-stats-" + suffix + ".md", Title: "Job Stats",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	// Create 2 pending jobs
	if err := testDB.CreateJob(ctx, fileID, "parse", 1); err != nil {
		t.Fatalf("CreateJob 1 failed: %v", err)
	}
	if err := testDB.CreateJob(ctx, fileID, "embed", 1); err != nil {
		t.Fatalf("CreateJob 2 failed: %v", err)
	}

	// Claim and complete one
	claimed, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(claimed) == 0 {
		t.Fatal("expected at least 1 claimed job")
	}
	jobID := models.MustRecordIDString(claimed[0].ID)
	if err := testDB.CompleteJob(ctx, jobID); err != nil {
		t.Fatalf("CompleteJob failed: %v", err)
	}

	since := time.Now().Add(-1 * time.Hour)
	stats, err := testDB.GetJobStats(ctx, since)
	if err != nil {
		t.Fatalf("GetJobStats failed: %v", err)
	}

	// At least 1 done and 1 pending (other tests may have left jobs behind)
	if stats.Done < 1 {
		t.Errorf("expected at least 1 done job, got %d", stats.Done)
	}
	if stats.Pending < 1 {
		t.Errorf("expected at least 1 pending job, got %d", stats.Pending)
	}
}

func TestGetJobStats_IncludesFailedCount(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-stats-fail-" + suffix + ".md", Title: "Job Stats Fail",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateJob(ctx, fileID, "embed", 1); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	claimed, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(claimed) == 0 {
		t.Fatal("expected at least 1 claimed job")
	}
	jobID := models.MustRecordIDString(claimed[0].ID)
	if err := testDB.FailJob(ctx, jobID, "test error"); err != nil {
		t.Fatalf("FailJob failed: %v", err)
	}

	since := time.Now().Add(-1 * time.Hour)
	stats, err := testDB.GetJobStats(ctx, since)
	if err != nil {
		t.Fatalf("GetJobStats failed: %v", err)
	}
	if stats.Failed < 1 {
		t.Errorf("expected at least 1 failed job, got %d", stats.Failed)
	}
}

func TestListRecentJobs(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-recent-" + suffix + ".md", Title: "Job Recent",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateJob(ctx, fileID, "parse", 1); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	jobs, err := testDB.ListRecentJobs(ctx, 10, []string{"pending"})
	if err != nil {
		t.Fatalf("ListRecentJobs failed: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least 1 recent pending job")
	}

	// Verify file_path is populated
	var found bool
	for _, j := range jobs {
		if j.FilePath != nil && *j.FilePath == "/job-recent-"+suffix+".md" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find job with matching file_path")
	}
}

func TestGetJobTypeDurations(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-durations-" + suffix + ".md", Title: "Job Durations",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateJob(ctx, fileID, "embed", 1); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Claim and complete
	claimed, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(claimed) == 0 {
		t.Fatal("expected at least 1 claimed job")
	}
	jobID := models.MustRecordIDString(claimed[0].ID)
	if err := testDB.CompleteJob(ctx, jobID); err != nil {
		t.Fatalf("CompleteJob failed: %v", err)
	}

	since := time.Now().Add(-1 * time.Hour)
	durations, err := testDB.GetJobTypeDurations(ctx, since)
	if err != nil {
		t.Fatalf("GetJobTypeDurations failed: %v", err)
	}

	// At least the "embed" type should have duration stats
	var found bool
	for _, d := range durations {
		if d.Type == "embed" {
			found = true
			if d.Count < 1 {
				t.Errorf("expected count >= 1 for embed, got %d", d.Count)
			}
			if d.MinMs < 0 {
				t.Errorf("expected min_ms >= 0, got %d", d.MinMs)
			}
			if d.MaxMs < d.MinMs {
				t.Errorf("expected max_ms >= min_ms, got max=%d min=%d", d.MaxMs, d.MinMs)
			}
			if d.AvgMs < float64(d.MinMs) || d.AvgMs > float64(d.MaxMs) {
				t.Errorf("expected min_ms <= avg_ms <= max_ms, got min=%d avg=%.1f max=%d", d.MinMs, d.AvgMs, d.MaxMs)
			}
			break
		}
	}
	if !found {
		t.Error("expected to find duration stats for 'embed' type")
	}
}

func TestGetJobTypeDurations_Empty(t *testing.T) {
	ctx := context.Background()

	// Query with a future "since" time so no jobs match
	future := time.Now().Add(1 * time.Hour)
	durations, err := testDB.GetJobTypeDurations(ctx, future)
	if err != nil {
		t.Fatalf("GetJobTypeDurations failed: %v", err)
	}
	if len(durations) != 0 {
		t.Errorf("expected 0 durations for future since, got %d", len(durations))
	}
}

func TestReconcileStaleRunningJobs(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/job-reconcile-" + suffix + ".md", Title: "Job Reconcile",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateJob(ctx, fileID, "embed", 1); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Claim job to put it in running state
	claimed, err := testDB.ClaimJobs(ctx, 1)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	if len(claimed) == 0 {
		t.Fatal("expected at least 1 job to be claimed")
	}

	// Reconcile: running jobs should be reset to pending
	count, err := testDB.ReconcileStaleRunningJobs(ctx)
	if err != nil {
		t.Fatalf("ReconcileStaleRunningJobs failed: %v", err)
	}
	if count == 0 {
		t.Error("expected at least 1 stale running job to be reconciled")
	}

	// After reconcile, job should be claimable again
	jobs, err := testDB.ClaimJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimJobs after reconcile failed: %v", err)
	}
	var found bool
	for _, j := range jobs {
		if models.MustRecordIDString(j.ID) == models.MustRecordIDString(claimed[0].ID) {
			found = true
			break
		}
	}
	if !found {
		t.Error("reconciled job should be claimable again")
	}
}

func TestCancelPendingJobs(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	vaultA := createTestVault(t, ctx, userID)
	vaultAID := models.MustRecordIDString(vaultA.ID)
	vaultB := createTestVault(t, ctx, userID)
	vaultBID := models.MustRecordIDString(vaultB.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())

	fileA1, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultAID, Path: "/cancel-a1-" + suffix + ".md", Title: "A1",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile A1 failed: %v", err)
	}
	fileA2, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultAID, Path: "/cancel-a2-" + suffix + ".md", Title: "A2",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile A2 failed: %v", err)
	}
	fileB1, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultBID, Path: "/cancel-b1-" + suffix + ".md", Title: "B1",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile B1 failed: %v", err)
	}

	fileA1ID := models.MustRecordIDString(fileA1.ID)
	fileA2ID := models.MustRecordIDString(fileA2.ID)
	fileB1ID := models.MustRecordIDString(fileB1.ID)

	if err := testDB.CreateJob(ctx, fileA1ID, "parse", 10); err != nil {
		t.Fatalf("CreateJob A1 failed: %v", err)
	}
	if err := testDB.CreateJob(ctx, fileA2ID, "parse", 10); err != nil {
		t.Fatalf("CreateJob A2 failed: %v", err)
	}
	if err := testDB.CreateJob(ctx, fileB1ID, "parse", 10); err != nil {
		t.Fatalf("CreateJob B1 failed: %v", err)
	}

	// Cancel only vault A jobs
	cancelled, err := testDB.CancelPendingJobs(ctx, vaultAID)
	if err != nil {
		t.Fatalf("CancelPendingJobs failed: %v", err)
	}
	if cancelled != 2 {
		t.Errorf("expected 2 cancelled, got %d", cancelled)
	}

	// Vault B job should still be claimable
	jobs, err := testDB.ClaimJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}
	var foundB bool
	for _, j := range jobs {
		if models.MustRecordIDString(j.File) == fileB1ID {
			foundB = true
		}
	}
	if !foundB {
		t.Error("vault B job should still be claimable after cancelling vault A")
	}
}

func TestCancelPendingJobs_AllVaults(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())

	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/cancel-all-" + suffix + ".md", Title: "All",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateJob(ctx, fileID, "parse", 10); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Cancel with empty vaultID (all vaults)
	cancelled, err := testDB.CancelPendingJobs(ctx, "")
	if err != nil {
		t.Fatalf("CancelPendingJobs (all) failed: %v", err)
	}
	if cancelled < 1 {
		t.Errorf("expected at least 1 cancelled, got %d", cancelled)
	}
}

func TestEnqueueReprocessJobs(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())

	_, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID:  vaultID,
		Path:     "/reprocess-" + suffix + ".md",
		Title:    "Markdown",
		Content:  "# Hello",
		Labels:   []string{},
		MimeType: "text/markdown",
	})
	if err != nil {
		t.Fatalf("CreateFile (md) failed: %v", err)
	}

	_, err = testDB.CreateFile(ctx, models.FileInput{
		VaultID:  vaultID,
		Path:     "/reprocess-" + suffix + ".pdf",
		Title:    "PDF",
		Content:  "",
		Labels:   []string{},
		MimeType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("CreateFile (pdf) failed: %v", err)
	}

	_, err = testDB.CreateFile(ctx, models.FileInput{
		VaultID:  vaultID,
		Path:     "/reprocess-" + suffix + ".mp3",
		Title:    "Audio",
		Content:  "",
		Labels:   []string{},
		MimeType: "audio/mpeg",
	})
	if err != nil {
		t.Fatalf("CreateFile (mp3) failed: %v", err)
	}

	count, err := testDB.EnqueueReprocessJobs(ctx, vaultID)
	if err != nil {
		t.Fatalf("EnqueueReprocessJobs failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 enqueued jobs, got %d", count)
	}

	// Claim all jobs and verify types
	jobs, err := testDB.ClaimJobs(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimJobs failed: %v", err)
	}

	typeSet := map[string]bool{}
	for _, j := range jobs {
		typeSet[j.Type] = true
	}
	for _, expected := range []string{"parse", "pdf", "transcribe"} {
		if !typeSet[expected] {
			t.Errorf("expected job type %q in claimed jobs", expected)
		}
	}
}
