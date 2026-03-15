package main

import (
	"fmt"
	"strings"

	"github.com/raphi011/know/internal/models"
	"github.com/spf13/cobra"
)

var (
	exportAPI     *apiFlags
	exportVaultID *string
	exportOutput  string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Download an export archive of all documents and assets in a vault",
	Long: `Creates a .tar.gz archive containing all documents and assets from a vault,
preserving the path structure.

Examples:
  know export --vault default
  know export --vault default -o my-export.tar.gz`,
	RunE: runExport,
}

func init() {
	exportAPI = addAPIFlags(exportCmd)
	exportVaultID = addVaultFlag(exportCmd, exportAPI)
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "output file path (default: know-export-{vault}.tar.gz)")
}

func runExport(cmd *cobra.Command, args []string) error {
	if exportOutput == "" {
		bareVault := strings.ReplaceAll(models.BareID("vault", *exportVaultID), "/", "_")
		exportOutput = fmt.Sprintf("know-export-%s.tar.gz", bareVault)
	}

	client := exportAPI.newClient()

	n, err := client.DownloadExport(cmd.Context(), *exportVaultID, exportOutput)
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	fmt.Printf("Export saved to %s (%s)\n", exportOutput, formatBytes(n))
	return nil
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
