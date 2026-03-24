package main

import (
	"fmt"
	"strings"

	"github.com/raphi011/know/internal/models"
	"github.com/spf13/cobra"
)

var (
	backupVaultID *string
	backupOutput  string
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a manifest-based backup of a vault",
	Long: `Creates a .tar.gz archive containing all documents, assets, and version
history from a vault. The archive includes a manifest.json with metadata
and raw files in their original path structure.

Unlike 'export', this format preserves labels, doc types, metadata,
folder settings, and version history — suitable for full vault restoration.

Examples:
  know backup --vault default
  know backup --vault default -o my-backup.tar.gz`,
	RunE: runBackup,
}

func init() {
	backupVaultID = addVaultFlag(backupCmd)
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "output file path (default: know-backup-{vault}.tar.gz)")
}

func runBackup(cmd *cobra.Command, args []string) error {
	if backupOutput == "" {
		bareVault := strings.ReplaceAll(models.BareID("vault", *backupVaultID), "/", "_")
		backupOutput = fmt.Sprintf("know-backup-%s.tar.gz", bareVault)
	}

	client := globalAPI.newClient()

	n, err := client.DownloadBackup(cmd.Context(), *backupVaultID, backupOutput)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	fmt.Printf("Backup saved to %s (%s)\n", backupOutput, formatBytes(n))
	return nil
}
