// Package main is a one-time bootstrap script that creates the initial
// user, vault, and API token directly against SurrealDB.
//
// Usage:
//
//	go run ./cmd/bootstrap --name "Admin" --email "admin@example.com"
//	go run ./cmd/bootstrap --name "Admin" --token kh_abc123...
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

func main() {
	name := flag.String("name", "admin", "user name")
	email := flag.String("email", "", "user email (optional)")
	token := flag.String("token", os.Getenv("KNOWHOW_BOOTSTRAP_TOKEN"), "API token to reuse (env: KNOWHOW_BOOTSTRAP_TOKEN)")
	userRecordID := flag.String("user-id", "admin", "stable user record ID")
	vaultRecordID := flag.String("vault-id", "default", "stable vault record ID")
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

	// 2. Create default vault with stable ID
	desc := "Default vault"
	vault, err := dbClient.CreateVaultWithID(ctx, *vaultRecordID, userID, models.VaultInput{
		Name:        "default",
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
