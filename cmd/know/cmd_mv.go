package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	mvAPI     *apiFlags
	mvVaultID *string
	mvDryRun  bool
)

var mvCmd = &cobra.Command{
	Use:   "mv <source> <destination>",
	Short: "Move a document or folder within a vault",
	Long: `Move a document or folder to a new path within the same vault.

Wiki-links referencing the old path are updated automatically.

Examples:
  know mv /docs/old-name.md /docs/new-name.md
  know mv /old-folder /new-folder
  know mv /docs/readme.md /archive/readme.md --vault my-vault
  know mv /docs/readme.md /archive/readme.md --dry-run`,
	Args: cobra.ExactArgs(2),
	RunE: runMv,
}

func init() {
	mvAPI = addAPIFlags(mvCmd)
	mvVaultID = addVaultFlag(mvCmd, mvAPI)
	mvCmd.Flags().BoolVar(&mvDryRun, "dry-run", false, "show what would be moved without changes")
	mvCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) >= 2 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return completeVaultPaths(mvAPI, mvVaultID, pathFilterAll)(cmd, args, toComplete)
	}
}

func runMv(_ *cobra.Command, args []string) error {
	source := args[0]
	destination := args[1]

	if !strings.HasPrefix(source, "/") {
		return fmt.Errorf("source path must start with /: %s", source)
	}
	if !strings.HasPrefix(destination, "/") {
		return fmt.Errorf("destination path must start with /: %s", destination)
	}

	client := mvAPI.newClient()
	result, err := client.Move(context.Background(), *mvVaultID, source, destination, mvDryRun)
	if err != nil {
		return fmt.Errorf("mv: %w", err)
	}

	if result.DryRun {
		fmt.Printf("Dry run: would move %s %s -> %s (%d documents)\n", result.Type, source, destination, result.Count)
	} else {
		fmt.Printf("Moved %s %s -> %s (%d documents)\n", result.Type, source, destination, result.Count)
	}

	return nil
}
