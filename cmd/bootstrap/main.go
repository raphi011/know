// Package main is a one-time bootstrap script that creates the initial
// user, vault, and API token directly against SurrealDB.
//
// Usage:
//
//	go run ./cmd/bootstrap --name "Admin" --email "admin@example.com"
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/raphaelgruber/memcp-go/internal/auth"
	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/models"
)

func main() {
	name := flag.String("name", "admin", "user name")
	email := flag.String("email", "", "user email (optional)")
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

	// 1. Create user
	var emailPtr *string
	if *email != "" {
		emailPtr = email
	}

	user, err := dbClient.CreateUser(ctx, models.UserInput{
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

	// 2. Create default vault
	desc := "Default vault"
	vault, err := dbClient.CreateVault(ctx, userID, models.VaultInput{
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

	// 3. Generate API token with access to the new vault
	rawToken, tokenHash, err := auth.GenerateToken()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: generate token: %v\n", err)
		os.Exit(1)
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
