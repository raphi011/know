package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/raphaelgruber/memcp-go/internal/client"
	"github.com/spf13/cobra"
)

var (
	usageSince    string
	usageDetailed bool
	usageCosts    bool
)

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show usage statistics",
	Long: `Show server runtime statistics and token usage for cost monitoring.

Examples:
  knowhow usage
  knowhow usage --since "7 days ago"
  knowhow usage --detailed
  knowhow usage --costs`,
	RunE: runUsage,
}

func init() {
	usageCmd.Flags().StringVar(&usageSince, "since", "24h", "time period (e.g., '24h', '7d', '30d')")
	usageCmd.Flags().BoolVar(&usageDetailed, "detailed", false, "show detailed breakdown")
	usageCmd.Flags().BoolVar(&usageCosts, "costs", false, "show cost estimates")
}

func runUsage(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Show server runtime stats
	stats, err := gqlClient.GetServerStats(ctx)
	if err != nil {
		return fmt.Errorf("get server stats: %w", err)
	}
	printServerStats(stats)
	fmt.Println()

	// Parse since duration
	var since time.Time
	switch usageSince {
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	default:
		// Try parsing as duration
		d, err := time.ParseDuration(usageSince)
		if err != nil {
			return fmt.Errorf("invalid duration: %s", usageSince)
		}
		since = time.Now().Add(-d)
	}

	sinceStr := since.Format(time.RFC3339)
	summary, err := gqlClient.GetUsageSummary(ctx, sinceStr)
	if err != nil {
		return fmt.Errorf("get token usage: %w", err)
	}

	fmt.Printf("Token Usage (since %s)\n", usageSince)
	fmt.Printf("═══════════════════════════════════════\n\n")

	fmt.Printf("Total tokens: %d\n", summary.TotalTokens)
	if usageCosts && summary.TotalCostUSD > 0 {
		fmt.Printf("Estimated cost: $%.4f\n", summary.TotalCostUSD)
	}

	if usageDetailed && len(summary.ByOperation) > 0 {
		fmt.Printf("\nBy Operation:\n")
		for op, tokensAny := range summary.ByOperation {
			tokens, ok := tokensAny.(float64)
			if !ok {
				continue
			}
			pct := 0.0
			if summary.TotalTokens > 0 {
				pct = tokens / float64(summary.TotalTokens) * 100
			}
			fmt.Printf("  %-15s %10.0f (%5.1f%%)\n", op, tokens, pct)
		}
	}

	if usageDetailed && len(summary.ByModel) > 0 {
		fmt.Printf("\nBy Model:\n")
		for model, tokensAny := range summary.ByModel {
			tokens, ok := tokensAny.(float64)
			if !ok {
				continue
			}
			pct := 0.0
			if summary.TotalTokens > 0 {
				pct = tokens / float64(summary.TotalTokens) * 100
			}
			fmt.Printf("  %-25s %10.0f (%5.1f%%)\n", model, tokens, pct)
		}
	}

	return nil
}

// printServerStats displays server runtime statistics.
func printServerStats(stats *client.ServerStats) {
	fmt.Printf("Server Statistics (in-memory, since restart)\n")
	fmt.Printf("═══════════════════════════════════════════════\n")
	fmt.Printf("Uptime: %.1f seconds\n", stats.UptimeSeconds)

	if stats.Embedding != nil {
		fmt.Printf("\nEmbeddings:\n")
		printOpStats(stats.Embedding)
	}

	if stats.LLMGenerate != nil {
		fmt.Printf("\nLLM Generate:\n")
		printOpStats(stats.LLMGenerate)
		printTokenStats(stats.LLMGenerate)
	}

	if stats.LLMStream != nil {
		fmt.Printf("\nLLM Stream:\n")
		printOpStats(stats.LLMStream)
		printTokenStats(stats.LLMStream)
	}

	if stats.DBQuery != nil {
		fmt.Printf("\nDB Query:\n")
		printOpStats(stats.DBQuery)
	}

	if stats.DBSearch != nil {
		fmt.Printf("\nDB Search:\n")
		printOpStats(stats.DBSearch)
	}
}

// printOpStats displays timing statistics for an operation.
func printOpStats(op *client.OperationStats) {
	fmt.Printf("  Calls: %d, Total: %dms\n", op.Count, op.TotalTimeMs)
	fmt.Printf("  Time: avg %.1fms, min %dms, max %dms\n",
		op.AvgTimeMs, op.MinTimeMs, op.MaxTimeMs)
}

// printTokenStats displays token statistics if available.
func printTokenStats(op *client.OperationStats) {
	if op.TotalInputTokens == nil || op.TotalOutputTokens == nil {
		return
	}
	fmt.Printf("  Tokens In:  %d total", *op.TotalInputTokens)
	if op.AvgInputTokens != nil {
		fmt.Printf(", avg %.0f", *op.AvgInputTokens)
	}
	if op.MinInputTokens != nil && op.MaxInputTokens != nil {
		fmt.Printf(", min %d, max %d", *op.MinInputTokens, *op.MaxInputTokens)
	}
	fmt.Println()

	fmt.Printf("  Tokens Out: %d total", *op.TotalOutputTokens)
	if op.AvgOutputTokens != nil {
		fmt.Printf(", avg %.0f", *op.AvgOutputTokens)
	}
	if op.MinOutputTokens != nil && op.MaxOutputTokens != nil {
		fmt.Printf(", min %d, max %d", *op.MinOutputTokens, *op.MaxOutputTokens)
	}
	fmt.Println()
}
