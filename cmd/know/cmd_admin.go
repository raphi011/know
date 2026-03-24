package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "System administration commands (requires system admin token)",
}

var adminCreateUserCmd = &cobra.Command{
	Use:   "create-user",
	Short: "Create a new user with a private vault",
	Long: `Create a new user with the given name and email address.
A private vault is automatically created for the user with admin membership.

The user can then log in via OIDC using their email address.

Examples:
  know admin create-user --name alice --email alice@example.com`,
	RunE: runAdminCreateUser,
}

var adminListUsersCmd = &cobra.Command{
	Use:   "list-users",
	Short: "List all users",
	Long: `List all users with their name, email, admin status, and OIDC provider.

Examples:
  know admin list-users`,
	RunE: runAdminListUsers,
}

func init() {
	f := adminCreateUserCmd.Flags()
	f.String("name", "", "user name (required)")
	f.String("email", "", "user email (required)")
	if err := adminCreateUserCmd.MarkFlagRequired("name"); err != nil {
		panic(fmt.Sprintf("mark name required: %v", err))
	}
	if err := adminCreateUserCmd.MarkFlagRequired("email"); err != nil {
		panic(fmt.Sprintf("mark email required: %v", err))
	}

	adminCmd.AddCommand(adminCreateUserCmd)
	adminCmd.AddCommand(adminListUsersCmd)
}

func runAdminCreateUser(cmd *cobra.Command, _ []string) error {
	name, _ := cmd.Flags().GetString("name")
	email, _ := cmd.Flags().GetString("email")

	client := globalAPI.newClient()
	ctx := context.Background()

	resp, err := client.AdminCreateUser(ctx, name, email)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Created user: %s (id: %s)\n", resp.User.Name, resp.User.ID)
	if resp.User.Email != nil {
		fmt.Fprintf(os.Stderr, "  Email: %s\n", *resp.User.Email)
	}
	fmt.Fprintf(os.Stderr, "Created private vault: %s (id: %s)\n", resp.Vault.Name, resp.Vault.ID)
	fmt.Fprintf(os.Stderr, "  Membership: admin\n")
	fmt.Fprintf(os.Stderr, "\nUser can now log in via OIDC with %s\n", email)
	return nil
}

func runAdminListUsers(_ *cobra.Command, _ []string) error {
	client := globalAPI.newClient()
	ctx := context.Background()

	users, err := client.AdminListUsers(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tEMAIL\tADMIN\tOIDC")
	for _, u := range users {
		email := "-"
		if u.Email != nil {
			email = *u.Email
		}
		admin := "no"
		if u.IsSystemAdmin {
			admin = "yes"
		}
		oidc := "-"
		if u.OIDCProvider != nil {
			oidc = *u.OIDCProvider
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.Name, email, admin, oidc)
	}
	return w.Flush()
}
