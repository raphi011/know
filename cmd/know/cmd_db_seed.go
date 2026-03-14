package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/models"
	"github.com/spf13/cobra"
)

var (
	seedName      string
	seedEmail     string
	seedToken     string
	seedUserID    string
	seedVaultID   string
	seedVaultName string
	seedNoAuth    bool
)

var dbSeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Apply schema and create user/vault/token (bootstrap the database)",
	RunE:  runDBSeed,
}

func init() {
	f := dbSeedCmd.Flags()
	f.StringVar(&seedName, "name", envOrDefault("KNOW_BOOTSTRAP_USER_NAME", "admin"), "user name")
	f.StringVar(&seedEmail, "email", os.Getenv("KNOW_BOOTSTRAP_USER_EMAIL"), "user email")
	f.StringVar(&seedToken, "new-token", os.Getenv("KNOW_BOOTSTRAP_TOKEN"), "API token to reuse")
	f.StringVar(&seedUserID, "user-id", envOrDefault("KNOW_BOOTSTRAP_USER_ID", "admin"), "stable user record ID")
	f.StringVar(&seedVaultID, "vault-id", envOrDefault("KNOW_BOOTSTRAP_VAULT_ID", "default"), "stable vault record ID")
	f.StringVar(&seedVaultName, "vault-name", envOrDefault("KNOW_BOOTSTRAP_VAULT_NAME", "default"), "vault display name")
	f.BoolVar(&seedNoAuth, "no-auth", os.Getenv("KNOW_NO_AUTH") == "true", "skip token creation")
}

func runDBSeed(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbClient, err := connectDB(ctx)
	if err != nil {
		return err
	}
	defer dbClient.Close(ctx) //nolint:errcheck // process exits immediately; close failure is benign

	if err := dbClient.InitSchema(ctx, embedDim); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	// 1. Create user with stable ID
	var emailPtr *string
	if seedEmail != "" {
		emailPtr = &seedEmail
	}

	user, err := dbClient.CreateUserWithID(ctx, seedUserID, models.UserInput{
		Name:  seedName,
		Email: emailPtr,
	})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		return fmt.Errorf("extract user ID: %w", err)
	}

	if err := dbClient.UpdateUserSystemAdmin(ctx, userID, true); err != nil {
		return fmt.Errorf("set system admin: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Created user: %s (id: %s, system_admin: true)\n", user.Name, userID)

	// 2. Create vault with stable ID
	desc := "Default vault"
	vault, err := dbClient.CreateVaultWithID(ctx, seedVaultID, userID, models.VaultInput{
		Name:        seedVaultName,
		Description: &desc,
	})
	if err != nil {
		return fmt.Errorf("create vault: %w", err)
	}

	vaultID, err := models.RecordIDString(vault.ID)
	if err != nil {
		return fmt.Errorf("extract vault ID: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Created vault: %s (id: %s)\n", vault.Name, vaultID)

	// 3. Create vault_member with admin role
	if _, err := dbClient.CreateVaultMember(ctx, userID, vaultID, models.RoleAdmin); err != nil {
		return fmt.Errorf("create vault member: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Created vault membership: user=%s, vault=%s, role=admin\n", userID, vaultID)

	// In no-auth mode, skip token creation
	if seedNoAuth {
		fmt.Fprintf(os.Stderr, "No-auth mode: skipping token creation\n")
		fmt.Fprintf(os.Stderr, "\nVault ID: %s\n", vaultID)
		return nil
	}

	// 4. Create API token
	var rawToken, tokenHash string
	if seedToken != "" {
		tokenHash, err = auth.UseToken(seedToken)
		if err != nil {
			return fmt.Errorf("invalid token: %w", err)
		}
		rawToken = seedToken
	} else {
		rawToken, tokenHash, err = auth.GenerateToken()
		if err != nil {
			return fmt.Errorf("generate token: %w", err)
		}
	}

	if _, err := dbClient.CreateToken(ctx, userID, tokenHash, "bootstrap"); err != nil {
		return fmt.Errorf("create token: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nAPI Token (save this — it will not be shown again):\n")
	fmt.Println(rawToken)
	fmt.Fprintf(os.Stderr, "\nVault ID: %s\n", vaultID)
	return nil
}
