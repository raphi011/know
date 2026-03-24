package main

import (
	"context"
	"fmt"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/spf13/cobra"
)

var (
	reprocessVault string
	reprocessYes   bool
)

var reprocessCmd = &cobra.Command{
	Use:   "reprocess",
	Short: "Reprocess all files (delete chunks, clear hashes, re-enqueue jobs)",
	Long: `Force reprocessing of all files in the pipeline.

This deletes all chunks, clears file content hashes, cancels pending jobs,
and enqueues fresh parse/pdf/transcribe jobs for every file.

The embed worker will automatically embed new chunks after processing.

Examples:
  know job reprocess --yes
  know job reprocess --vault default --yes`,
	RunE: runReprocess,
}

func init() {
	reprocessCmd.Flags().StringVar(&reprocessVault, "vault", "", "only reprocess files in this vault")
	reprocessCmd.Flags().BoolVarP(&reprocessYes, "yes", "y", false, "skip confirmation prompt")
	if err := reprocessCmd.RegisterFlagCompletionFunc("vault", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register vault completion: %v", err))
	}
}

func runReprocess(_ *cobra.Command, _ []string) error {
	if !reprocessYes {
		scope := "all vaults"
		if reprocessVault != "" {
			scope = fmt.Sprintf("vault %q", reprocessVault)
		}
		fmt.Printf("This will delete all chunks and reprocess all files in %s.\nContinue? [y/N] ", scope)
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil || (answer != "y" && answer != "Y") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	client := globalAPI.newClient()
	ctx := context.Background()

	resp, err := client.Reprocess(ctx, apiclient.ReprocessRequest{
		Vault: reprocessVault,
	})
	if err != nil {
		return fmt.Errorf("reprocess: %w", err)
	}

	fmt.Printf("Reprocess complete:\n")
	fmt.Printf("  Jobs cancelled: %d\n", resp.JobsCancelled)
	fmt.Printf("  Hashes cleared: %d\n", resp.HashesCleared)
	fmt.Printf("  Jobs enqueued:  %d\n", resp.JobsEnqueued)
	return nil
}
