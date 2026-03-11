package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/raphi011/knowhow/internal/apiclient"
	"github.com/spf13/cobra"
)

var (
	labelsVaultID string
	labelsCounts  bool
)

var labelsCmd = &cobra.Command{
	Use:   "labels",
	Short: "List labels in a vault",
	Long: `List all labels used by documents in a vault.

Examples:
  knowhow labels --vault default
  knowhow labels --vault default --count`,
	RunE: runLabels,
}

func init() {
	labelsCmd.Flags().StringVar(&labelsVaultID, "vault", "", "vault name (required)")
	labelsCmd.Flags().BoolVar(&labelsCounts, "count", false, "show document count per label")
	if err := labelsCmd.MarkFlagRequired("vault"); err != nil {
		panic(fmt.Sprintf("mark vault flag required: %v", err))
	}
}

func runLabels(_ *cobra.Command, _ []string) error {
	if err := requireToken(); err != nil {
		return fmt.Errorf("labels: %w", err)
	}

	client := apiclient.New(apiURL, apiToken)
	ctx := context.Background()

	if labelsCounts {
		counts, err := client.ListLabelsWithCounts(ctx, labelsVaultID)
		if err != nil {
			return fmt.Errorf("labels: %w", err)
		}
		for _, lc := range counts {
			fmt.Printf("%s (%d)\n", lc.Label, lc.Count)
		}
		return nil
	}

	labels, err := client.ListLabels(ctx, labelsVaultID)
	if err != nil {
		return fmt.Errorf("labels: %w", err)
	}
	sort.Strings(labels)
	for _, l := range labels {
		fmt.Println(l)
	}
	return nil
}
