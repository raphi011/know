package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/raphi011/know/internal/db"
	"github.com/spf13/cobra"
)

var (
	dbURL       string
	dbNamespace string
	dbDatabase  string
	dbUser      string
	dbPass      string
	dbAuthLevel string
	embedDim    int
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database commands (direct SurrealDB access)",
}

func init() {
	pf := dbCmd.PersistentFlags()
	pf.StringVar(&dbURL, "db-url", envOrDefault("SURREALDB_URL", "ws://localhost:4002/rpc"), "SurrealDB URL")
	pf.StringVar(&dbNamespace, "db-namespace", envOrDefault("SURREALDB_NAMESPACE", "knowledge"), "SurrealDB namespace")
	pf.StringVar(&dbDatabase, "db-database", envOrDefault("SURREALDB_DATABASE", "graph"), "SurrealDB database")
	pf.StringVar(&dbUser, "db-user", envOrDefault("SURREALDB_USER", "root"), "SurrealDB user")
	pf.StringVar(&dbPass, "db-pass", envOrDefault("SURREALDB_PASS", "root"), "SurrealDB password")
	pf.StringVar(&dbAuthLevel, "db-auth-level", envOrDefault("SURREALDB_AUTH_LEVEL", "root"), "SurrealDB auth level")
	pf.IntVar(&embedDim, "embed-dimension", envOrDefaultInt("KNOW_EMBED_DIMENSION", 768), "embedding vector dimension")

	dbCmd.AddCommand(dbSeedCmd)
	dbCmd.AddCommand(dbWipeCmd)
}

func connectDB(ctx context.Context) (*db.Client, error) {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	client, err := db.NewClient(ctx, db.Config{
		URL:       dbURL,
		Namespace: dbNamespace,
		Database:  dbDatabase,
		Username:  dbUser,
		Password:  dbPass,
		AuthLevel: dbAuthLevel,
	}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to DB: %w", err)
	}
	return client, nil
}
