package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/spf13/cobra"
)

var (
	jobSince string
	jobJSON  bool
)

var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "Show pipeline job status and statistics",
	Long: `Display the current state of the ingestion pipeline.

Shows job counts by status, per-type duration statistics (min/max/avg),
currently active jobs, and recent failures.

Examples:
  know job
  know job --since 1h
  know job --since 7d
  know job --json`,
	RunE: runJob,
}

func init() {
	jobCmd.Flags().StringVar(&jobSince, "since", "24h", "time window for stats (e.g. 1h, 24h, 7d)")
	jobCmd.Flags().BoolVar(&jobJSON, "json", false, "output as JSON")
	if err := jobCmd.RegisterFlagCompletionFunc("since", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register since completion: %v", err))
	}
}

func runJob(_ *cobra.Command, _ []string) error {
	client := globalAPI.newClient()
	ctx := context.Background()

	resp, err := client.GetJobStatus(ctx, jobSince)
	if err != nil {
		return fmt.Errorf("job: %w", err)
	}

	if jobJSON {
		out, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return fmt.Errorf("job: marshal json: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	printJobStatus(resp)
	return nil
}

func printJobStatus(resp *apiclient.JobStatusResponse) {
	s := resp.Stats

	fmt.Printf("Pipeline Status (last %s)\n", jobSince)
	fmt.Println("──────────────────────────")
	fmt.Printf("%-12s %d\n", "Pending:", s.Pending)
	fmt.Printf("%-12s %d\n", "Running:", s.Running)
	fmt.Printf("%-12s %d\n", "Done:", s.Done)
	fmt.Printf("%-12s %d\n", "Failed:", s.Failed)
	fmt.Printf("%-12s %d\n", "Cancelled:", s.Cancelled)

	if len(resp.Durations) > 0 {
		sort.Slice(resp.Durations, func(i, j int) bool {
			return resp.Durations[i].Type < resp.Durations[j].Type
		})

		fmt.Println()
		fmt.Println("Duration by Type (completed)")
		fmt.Println("────────────────────────────")
		fmt.Printf("%-14s %6s %10s %10s %10s\n", "TYPE", "COUNT", "MIN", "MAX", "AVG")
		for _, d := range resp.Durations {
			fmt.Printf("%-14s %6d %10s %10s %10s\n",
				d.Type,
				d.Count,
				formatMs(d.MinMs),
				formatMs(d.MaxMs),
				formatMs(int64(d.AvgMs)),
			)
		}
	}

	if len(resp.Active) > 0 {
		fmt.Println()
		fmt.Printf("Active Jobs (%d)\n", len(resp.Active))
		fmt.Println("────────────────")
		fmt.Printf("%-14s %-40s %s\n", "TYPE", "FILE", "STARTED")
		for _, j := range resp.Active {
			filePath := "-"
			if j.FilePath != nil {
				filePath = *j.FilePath
			}
			started := "-"
			if j.StartedAt != nil {
				started = formatTimeAgo(*j.StartedAt)
			}
			fmt.Printf("%-14s %-40s %s\n", j.Type, truncate(filePath, 40), started)
		}
	}

	if len(resp.RecentFailed) > 0 {
		fmt.Println()
		fmt.Printf("Recent Failures (%d)\n", len(resp.RecentFailed))
		fmt.Println("────────────────────")
		fmt.Printf("%-14s %-30s %-30s %s\n", "TYPE", "FILE", "ERROR", "WHEN")
		for _, j := range resp.RecentFailed {
			filePath := "-"
			if j.FilePath != nil {
				filePath = *j.FilePath
			}
			errMsg := "-"
			if j.Error != nil {
				errMsg = *j.Error
			}
			when := formatTimeAgo(j.CreatedAt)
			fmt.Printf("%-14s %-30s %-30s %s\n",
				j.Type,
				truncate(filePath, 30),
				truncate(errMsg, 30),
				when,
			)
		}
	}

	total := s.Pending + s.Running + s.Done + s.Failed + s.Cancelled
	if total == 0 {
		fmt.Println()
		fmt.Println("No jobs found in the given time window.")
	}
}

// formatMs formats milliseconds as a human-readable duration.
func formatMs(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	if ms < 60_000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%.1fm", float64(ms)/60_000)
}

// formatTimeAgo formats a time as a human-readable "ago" string.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// truncate shortens a string to maxLen, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}
