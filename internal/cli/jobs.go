package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var jobsCmd = &cobra.Command{
	Use:   "jobs [job-id]",
	Short: "List or inspect background jobs",
	Long: `List all background jobs or inspect a specific job by ID.

Examples:
  knowhow jobs           # List all jobs
  knowhow jobs abc123    # Show details for job abc123`,
	Args: cobra.MaximumNArgs(1),
	RunE: runJobs,
}

func init() {
	rootCmd.AddCommand(jobsCmd)
}

func runJobs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// If job ID provided, show that specific job
	if len(args) == 1 {
		return showJob(ctx, args[0])
	}

	// List all jobs
	return listJobs(ctx)
}

func listJobs(ctx context.Context) error {
	jobs, err := gqlClient.ListJobs(ctx)
	if err != nil {
		return fmt.Errorf("list jobs: %w", err)
	}

	if len(jobs) == 0 {
		fmt.Println("No jobs found")
		return nil
	}

	fmt.Printf("%-10s %-10s %-12s %-10s %s\n", "ID", "TYPE", "STATUS", "PROGRESS", "STARTED")
	fmt.Println("------------------------------------------------------------------------")

	for _, job := range jobs {
		progress := ""
		if job.Total > 0 {
			progress = fmt.Sprintf("%d/%d", job.Progress, job.Total)
		}
		started := job.StartedAt.Format("15:04:05")
		fmt.Printf("%-10s %-10s %-12s %-10s %s\n", job.ID, job.Type, job.Status, progress, started)
	}

	return nil
}

func showJob(ctx context.Context, id string) error {
	job, err := gqlClient.GetJob(ctx, id)
	if err != nil {
		return fmt.Errorf("get job: %w", err)
	}
	if job == nil {
		return fmt.Errorf("job not found: %s", id)
	}

	fmt.Printf("Job: %s\n", job.ID)
	fmt.Printf("  Type: %s\n", job.Type)
	fmt.Printf("  Status: %s\n", job.Status)
	if job.Total > 0 {
		fmt.Printf("  Progress: %d/%d\n", job.Progress, job.Total)
	}
	fmt.Printf("  Started: %s\n", job.StartedAt.Format(time.RFC3339))
	if job.CompletedAt != nil {
		fmt.Printf("  Completed: %s\n", job.CompletedAt.Format(time.RFC3339))
		duration := job.CompletedAt.Sub(job.StartedAt)
		fmt.Printf("  Duration: %s\n", duration.Round(time.Second))
	}

	if job.Error != nil && *job.Error != "" {
		fmt.Printf("  Error: %s\n", *job.Error)
	}

	if job.Result != nil {
		fmt.Println("\nResult:")
		fmt.Printf("  Files processed: %d\n", job.Result.FilesProcessed)
		fmt.Printf("  Entities created: %d\n", job.Result.EntitiesCreated)
		fmt.Printf("  Chunks created: %d\n", job.Result.ChunksCreated)
		if job.Result.RelationsCreated > 0 {
			fmt.Printf("  Relations created: %d\n", job.Result.RelationsCreated)
		}
		if len(job.Result.Errors) > 0 {
			fmt.Printf("\n  Errors (%d):\n", len(job.Result.Errors))
			for _, e := range job.Result.Errors {
				fmt.Printf("    - %s\n", e)
			}
		}
	}

	return nil
}
