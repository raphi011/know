package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	restoreAPI *apiFlags
)

var restoreCmd = &cobra.Command{
	Use:   "restore <archive>",
	Short: "Restore a vault from a backup archive",
	Long: `Restores a vault from a manifest-based backup archive (.tar.gz).
Creates the vault if it doesn't exist, restores all files and version history.
Chunks and embeddings are regenerated via the normal pipeline.

Examples:
  know restore my-backup.tar.gz
  know restore know-backup-default.tar.gz`,
	Args: cobra.ExactArgs(1),
	RunE: runRestore,
}

func init() {
	restoreAPI = addAPIFlags(restoreCmd)
}

func runRestore(cmd *cobra.Command, args []string) error {
	archivePath := args[0]

	client := restoreAPI.newClient()

	if err := client.RestoreBackup(cmd.Context(), archivePath); err != nil {
		return fmt.Errorf("restore: %w", err)
	}

	fmt.Printf("Restore complete from %s\n", archivePath)
	return nil
}
