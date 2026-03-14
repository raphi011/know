package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	rmAPI       *apiFlags
	rmVaultID   *string
	rmRecursive bool
	rmDryRun    bool
)

var rmCmd = &cobra.Command{
	Use:   "rm <path>",
	Short: "Remove documents or folders from a vault",
	Long: `Remove a document or folder from a vault.

Removing a folder requires the -r/--recursive flag, similar to Unix rm.

Environment variables:
  KNOW_VAULT    vault name (alternative to --vault flag)

Examples:
  know rm /docs/readme.md
  know rm /docs/readme.md --vault my-vault
  know rm /docs/readme.md --dry-run
  know rm /docs -r
  know rm /docs -r --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runRm,
}

func init() {
	rmAPI = addAPIFlags(rmCmd)
	rmVaultID = addVaultFlag(rmCmd)
	rmCmd.Flags().BoolVarP(&rmRecursive, "recursive", "r", false, "recursively delete folder contents")
	rmCmd.Flags().BoolVar(&rmDryRun, "dry-run", false, "show what would be deleted without deleting")
}

func runRm(_ *cobra.Command, args []string) error {
	path := args[0]

	client := rmAPI.newClient()
	result, err := client.DeleteDocuments(context.Background(), *rmVaultID, path, rmRecursive, rmDryRun)
	if err != nil {
		return fmt.Errorf("rm: %w", err)
	}

	for _, d := range result.Deleted {
		fmt.Printf("- %s\n", d)
	}

	if result.DryRun {
		fmt.Printf("\nDry run: %d would be removed\n", result.Count)
	} else {
		fmt.Printf("\nRemoved: %d\n", result.Count)
	}

	return nil
}
