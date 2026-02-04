package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var (
	usageSince    string
	usageDetailed bool
	usageCosts    bool
)

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show token usage statistics",
	Long: `Show LLM token usage statistics for cost monitoring.

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
