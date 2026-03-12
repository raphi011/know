package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	vaultInfoAPI     *apiFlags
	vaultInfoVaultID *string
)

var vaultCmd = &cobra.Command{
	Use:   "vault [name]",
	Short: "Show vault info and stats",
	Long: `Show comprehensive stats about a vault including document counts,
chunk/embedding status, labels, members, assets, wiki-links, and token usage.

The vault name can be passed as a positional argument or via --vault flag.

Environment variables:
  KNOWHOW_VAULT    vault name (alternative to --vault flag)

Examples:
  knowhow vault
  knowhow vault default
  knowhow vault my-vault --api-url http://localhost:4001`,
	Args: cobra.MaximumNArgs(1),
	RunE: runVault,
}

func init() {
	vaultInfoAPI = addAPIFlags(vaultCmd)
	vaultInfoVaultID = addVaultFlag(vaultCmd)
}

func runVault(_ *cobra.Command, args []string) error {
	vaultName := *vaultInfoVaultID
	if len(args) > 0 {
		vaultName = args[0]
	}

	client := vaultInfoAPI.newClient()
	ctx := context.Background()

	info, err := client.GetVaultInfo(ctx, vaultName)
	if err != nil {
		return fmt.Errorf("vault: %w", err)
	}

	fmt.Printf("Vault: %s\n", info.Name)
	if info.Description != nil {
		fmt.Printf("Description: %s\n", *info.Description)
	}
	fmt.Printf("Created: %s\n", info.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", info.UpdatedAt.Format("2006-01-02 15:04:05"))

	fmt.Println()
	fmt.Println("Documents")
	fmt.Printf("  Total:        %s\n", formatNumber(int64(info.DocumentCount)))
	fmt.Printf("  Unprocessed:  %s\n", formatNumber(int64(info.UnprocessedDocs)))

	fmt.Println()
	fmt.Println("Chunks")
	fmt.Printf("  Total:        %s\n", formatNumber(int64(info.ChunkTotal)))
	if info.ChunkTotal > 0 {
		pct := info.ChunkWithEmbedding * 100 / info.ChunkTotal
		fmt.Printf("  Embedded:     %s (%d%%)\n", formatNumber(int64(info.ChunkWithEmbedding)), pct)
	} else {
		fmt.Printf("  Embedded:     %s\n", formatNumber(int64(info.ChunkWithEmbedding)))
	}
	fmt.Printf("  Pending:      %s\n", formatNumber(int64(info.ChunkPending)))

	fmt.Println()
	fmt.Printf("Labels (%d)\n", info.LabelCount)
	for _, l := range info.TopLabels {
		fmt.Printf("  %-12s (%d)\n", l.Name, l.Count)
	}

	if len(info.Members) > 0 {
		fmt.Println()
		fmt.Println("Members")
		for _, m := range info.Members {
			fmt.Printf("  %-12s %s\n", m.Name, m.Role)
		}
	}

	fmt.Println()
	fmt.Println("Assets")
	fmt.Printf("  Count:        %s\n", formatNumber(int64(info.AssetCount)))
	fmt.Printf("  Total size:   %s\n", formatBytes(info.AssetTotalSize))

	fmt.Println()
	fmt.Println("Wiki-links")
	fmt.Printf("  Total:        %s\n", formatNumber(int64(info.WikiLinkTotal)))
	fmt.Printf("  Broken:       %s\n", formatNumber(int64(info.WikiLinkBroken)))

	fmt.Println()
	fmt.Printf("Templates:      %s\n", formatNumber(int64(info.TemplateCount)))
	fmt.Printf("Versions:       %s\n", formatNumber(int64(info.VersionCount)))
	fmt.Printf("Conversations:  %s\n", formatNumber(int64(info.ConversationCount)))
	fmt.Printf("Token usage:    %s in / %s out\n", formatNumber(info.TokenInput), formatNumber(info.TokenOutput))

	return nil
}

func formatNumber(n int64) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	case n < 1_000_000_000:
		return fmt.Sprintf("%d,%03d,%03d", n/1_000_000, (n/1000)%1000, n%1000)
	default:
		return fmt.Sprintf("%d,%03d,%03d,%03d", n/1_000_000_000, (n/1_000_000)%1000, (n/1000)%1000, n%1000)
	}
}
