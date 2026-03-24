package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var devYes bool

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Development commands (requires system admin token)",
}

var devResetDBCmd = &cobra.Command{
	Use:   "reset-db",
	Short: "Reset database schema while preserving users, vaults, and tokens",
	Long: `Drops all tables and re-applies the schema from scratch.
Users, vaults, vault memberships, and API tokens are preserved.
All other data (documents, conversations, etc.) is destroyed.

This is useful during development when the schema changes.`,
	RunE: runDevResetDB,
}

func init() {
	devResetDBCmd.Flags().BoolVarP(&devYes, "yes", "y", false, "skip confirmation prompt")
	devCmd.AddCommand(devResetDBCmd)
}

func runDevResetDB(_ *cobra.Command, _ []string) error {
	if !devYes {
		fmt.Fprint(os.Stderr, "This will destroy all documents, conversations, and other data. Proceed? [y/N] ")
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil {
			return fmt.Errorf("read confirmation: %w (use --yes to skip)", err)
		}
		if !strings.EqualFold(strings.TrimSpace(answer), "y") {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	client := globalAPI.newClient()
	ctx := context.Background()

	resp, err := client.DevResetDB(ctx)
	if err != nil {
		return err
	}

	p := resp.Preserved
	fmt.Fprintf(os.Stderr, "Database reset complete.\n")
	fmt.Fprintf(os.Stderr, "Preserved: %d users, %d vaults, %d members, %d tokens\n",
		p.Users, p.Vaults, p.VaultMembers, p.Tokens)
	return nil
}
