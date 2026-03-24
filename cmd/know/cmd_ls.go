package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	lsVaultID   *string
	lsRecursive bool
)

var lsCmd = &cobra.Command{
	Use:   "ls [path]",
	Short: "List files and folders in a vault",
	Long: `List files and folders in a vault path, similar to the Unix ls command.

By default only shows direct children of the given path. Use -R for recursive listing.

Environment variables:
  KNOW_VAULT    vault name (alternative to --vault flag)

Examples:
  know ls
  know ls /docs
  know ls /docs -R
  know ls --vault my-vault /notes`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLs,
}

func init() {
	lsVaultID = addVaultFlag(lsCmd)
	lsCmd.Flags().BoolVarP(&lsRecursive, "recursive", "R", false, "list files recursively")
	lsCmd.ValidArgsFunction = completeVaultPaths(lsVaultID, pathFilterFolders)
}

func runLs(_ *cobra.Command, args []string) error {
	path := "/"
	if len(args) > 0 {
		path = args[0]
	}

	client := globalAPI.newClient()
	entries, err := client.ListFiles(context.Background(), *lsVaultID, path, lsRecursive)
	if err != nil {
		return fmt.Errorf("ls: %w", err)
	}

	for _, e := range entries {
		name := e.Name
		if lsRecursive {
			name = e.Path
		}
		if e.IsDir {
			name += "/"
		}
		fmt.Println(name)
	}

	return nil
}
