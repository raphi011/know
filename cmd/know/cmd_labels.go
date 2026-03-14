package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var (
	labelsAPI     *apiFlags
	labelsVaultID *string
	labelsCounts  bool
)

var labelsCmd = &cobra.Command{
	Use:   "labels",
	Short: "List labels in a vault",
	Long: `List all labels used by documents in a vault.

Environment variables:
  KNOW_VAULT    vault name (alternative to --vault flag)

Examples:
  know labels --vault default
  know labels --vault default --count`,
	RunE: runLabels,
}

func init() {
	labelsAPI = addAPIFlags(labelsCmd)
	labelsVaultID = addVaultFlag(labelsCmd)
	labelsCmd.Flags().BoolVar(&labelsCounts, "count", false, "show document count per label")
}

func runLabels(_ *cobra.Command, _ []string) error {
	client := labelsAPI.newClient()
	ctx := context.Background()

	if labelsCounts {
		counts, err := client.ListLabelsWithCounts(ctx, *labelsVaultID)
		if err != nil {
			return fmt.Errorf("labels: %w", err)
		}
		for _, lc := range counts {
			fmt.Printf("%s (%d)\n", lc.Label, lc.Count)
		}
		return nil
	}

	labels, err := client.ListLabels(ctx, *labelsVaultID)
	if err != nil {
		return fmt.Errorf("labels: %w", err)
	}
	sort.Strings(labels)
	for _, l := range labels {
		fmt.Println(l)
	}
	return nil
}
