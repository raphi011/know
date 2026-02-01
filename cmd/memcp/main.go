// Package main provides the entry point for the memcp MCP server.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/embedding"
	"github.com/raphaelgruber/memcp-go/internal/server"
	"github.com/raphaelgruber/memcp-go/internal/tools"
)

const version = "0.1.0"

func main() {
	// Load configuration
	cfg := config.Load()

	// Setup logger (dual output: stderr text + file JSON)
	logger, cleanup := config.SetupLogger(cfg.LogFile, cfg.LogLevel)
	defer cleanup()

	// Log startup info
	logger.Info("memcp starting",
		"version", version,
		"surrealdb_url", cfg.SurrealDBURL,
		"embedding_model", cfg.EmbeddingModel,
	)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		logger.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	// Connect to database
	dbCfg := db.Config{
		URL:       cfg.SurrealDBURL,
		Namespace: cfg.SurrealDBNamespace,
		Database:  cfg.SurrealDBDatabase,
		Username:  cfg.SurrealDBUser,
		Password:  cfg.SurrealDBPass,
		AuthLevel: cfg.SurrealDBAuthLevel,
	}

	dbClient, err := db.NewClient(ctx, dbCfg, logger)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer func() {
		logger.Info("closing database connection")
		_ = dbClient.Close(ctx)
	}()

	// Initialize database schema
	if err := dbClient.InitSchema(ctx); err != nil {
		logger.Error("failed to initialize database schema", "error", err)
		os.Exit(1)
	}

	// Create embedder
	embedder, err := embedding.DefaultOllama()
	if err != nil {
		logger.Error("failed to create embedder", "error", err)
		os.Exit(1)
	}
	logger.Info("embedder initialized", "model", embedder.Model())

	// Create and setup server
	srv := server.New(version, logger)
	srv.Setup()

	// Register tools
	deps := &tools.Dependencies{
		DB:       dbClient,
		Embedder: embedder,
		Logger:   logger,
	}
	tools.RegisterAll(srv.MCPServer(), deps)
	logger.Info("tools registered", "count", 1)

	// Log ready state
	logger.Info("server ready, awaiting connections")

	// Run server (blocks until disconnect or context cancelled)
	if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	logger.Info("shutdown complete")
}
