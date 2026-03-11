package main

import (
	"fmt"
	"strings"

	"github.com/raphi011/knowhow/internal/apiclient"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/spf13/cobra"
)

var (
	backupVaultID string
	backupOutput  string
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Download a backup archive of all documents and assets in a vault",
	Long: `Creates a .tar.gz archive containing all documents and assets from a vault,
preserving the path structure.

Examples:
  knowhow backup --vault vault:default
  knowhow backup --vault vault:default -o my-backup.tar.gz`,
	RunE: runBackup,
}

func init() {
	backupCmd.Flags().StringVar(&backupVaultID, "vault", "", "vault ID (required)")
	backupCmd.Flags().StringVarP(&backupOutput, "output", "o", "", "output file path (default: knowhow-backup-{vault}.tar.gz)")
	if err := backupCmd.MarkFlagRequired("vault"); err != nil {
		panic(fmt.Sprintf("mark vault flag required: %v", err))
	}
}

func runBackup(cmd *cobra.Command, args []string) error {
	if backupOutput == "" {
		bareVault := strings.ReplaceAll(models.BareID("vault", backupVaultID), "/", "_")
		backupOutput = fmt.Sprintf("knowhow-backup-%s.tar.gz", bareVault)
	}

	client := apiclient.New(apiURL, apiToken)

	n, err := client.DownloadBackup(cmd.Context(), backupVaultID, backupOutput)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}

	fmt.Printf("Backup saved to %s (%s)\n", backupOutput, formatBytes(n))
	return nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
