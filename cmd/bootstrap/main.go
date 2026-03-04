// Package main is a one-time bootstrap script that creates the initial
// user, vault, and API token directly against SurrealDB.
//
// All flags fall back to environment variables, so `just bootstrap` works
// with zero arguments when the justfile exports the defaults.
//
// Usage:
//
//	go run ./cmd/bootstrap --name "Admin" --email "admin@example.com"
//	go run ./cmd/bootstrap --name "Admin" --token kh_abc123...
//	just bootstrap  # uses env vars from justfile
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/config"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	name := flag.String("name", envOrDefault("KNOWHOW_BOOTSTRAP_USER_NAME", "admin"), "user name (env: KNOWHOW_BOOTSTRAP_USER_NAME)")
	email := flag.String("email", os.Getenv("KNOWHOW_BOOTSTRAP_USER_EMAIL"), "user email (env: KNOWHOW_BOOTSTRAP_USER_EMAIL)")
	token := flag.String("token", os.Getenv("KNOWHOW_BOOTSTRAP_TOKEN"), "API token to reuse (env: KNOWHOW_BOOTSTRAP_TOKEN)")
	userRecordID := flag.String("user-id", envOrDefault("KNOWHOW_BOOTSTRAP_USER_ID", "admin"), "stable user record ID (env: KNOWHOW_BOOTSTRAP_USER_ID)")
	vaultRecordID := flag.String("vault-id", envOrDefault("KNOWHOW_BOOTSTRAP_VAULT_ID", "default"), "stable vault record ID (env: KNOWHOW_BOOTSTRAP_VAULT_ID)")
	vaultName := flag.String("vault-name", envOrDefault("KNOWHOW_BOOTSTRAP_VAULT_NAME", "default"), "vault display name (env: KNOWHOW_BOOTSTRAP_VAULT_NAME)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbClient, err := db.NewClient(ctx, db.Config{
		URL:       cfg.SurrealDBURL,
		Namespace: cfg.SurrealDBNamespace,
		Database:  cfg.SurrealDBDatabase,
		Username:  cfg.SurrealDBUser,
		Password:  cfg.SurrealDBPass,
		AuthLevel: cfg.SurrealDBAuthLevel,
	}, nil, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: connect to DB: %v\n", err)
		os.Exit(1)
	}
	defer dbClient.Close(ctx)

	if err := dbClient.InitSchema(ctx, cfg.EmbedDimension); err != nil {
		fmt.Fprintf(os.Stderr, "Error: init schema: %v\n", err)
		os.Exit(1)
	}

	// 0. Wipe existing data
	if err := dbClient.WipeData(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: wipe data: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Wiped existing data\n")

	// 1. Create user with stable ID
	var emailPtr *string
	if *email != "" {
		emailPtr = email
	}

	user, err := dbClient.CreateUserWithID(ctx, *userRecordID, models.UserInput{
		Name:  *name,
		Email: emailPtr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: create user: %v\n", err)
		os.Exit(1)
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: extract user ID: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Created user: %s (id: %s)\n", user.Name, userID)

	// 2. Create vault with stable ID
	desc := "Default vault"
	vault, err := dbClient.CreateVaultWithID(ctx, *vaultRecordID, userID, models.VaultInput{
		Name:        *vaultName,
		Description: &desc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: create vault: %v\n", err)
		os.Exit(1)
	}

	vaultID, err := models.RecordIDString(vault.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: extract vault ID: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Created vault: %s (id: %s)\n", vault.Name, vaultID)

	// In no-auth mode, skip token creation — user + vault are enough.
	if cfg.NoAuth {
		fmt.Fprintf(os.Stderr, "No-auth mode: skipping token creation\n")
		fmt.Fprintf(os.Stderr, "\nVault ID: %s\n", vaultID)
		return
	}

	// 3. Create API token with access to the new vault
	var rawToken, tokenHash string
	if *token != "" {
		tokenHash, err = auth.UseToken(*token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid token: %v\n", err)
			os.Exit(1)
		}
		rawToken = *token
	} else {
		rawToken, tokenHash, err = auth.GenerateToken()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: generate token: %v\n", err)
			os.Exit(1)
		}
	}

	if _, err := dbClient.CreateToken(ctx, userID, tokenHash, "bootstrap", []string{vaultID}); err != nil {
		fmt.Fprintf(os.Stderr, "Error: create token: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\nAPI Token (save this — it will not be shown again):\n")
	// Print raw token to stdout so it can be captured by scripts
	fmt.Println(rawToken)
	fmt.Fprintf(os.Stderr, "\nVault ID: %s\n", vaultID)
}
