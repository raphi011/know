// Package graph provides GraphQL resolvers for Knowhow.
// This file will not be regenerated automatically.
package graph

import (
	"context"
	"log/slog"

	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/document"
	"github.com/raphaelgruber/memcp-go/internal/llm"
	"github.com/raphaelgruber/memcp-go/internal/search"
	"github.com/raphaelgruber/memcp-go/internal/template"
	"github.com/raphaelgruber/memcp-go/internal/vault"
)

// Resolver is the root resolver with all dependencies.
type Resolver struct {
	db              *db.Client
	vaultService    *vault.Service
	documentService *document.Service
	searchService   *search.Service
	templateService *template.Service
}

// NewResolver creates a new resolver with all dependencies.
func NewResolver(ctx context.Context, cfg config.Config) (*Resolver, error) {
	dbCfg := db.Config{
		URL:       cfg.SurrealDBURL,
		Namespace: cfg.SurrealDBNamespace,
		Database:  cfg.SurrealDBDatabase,
		Username:  cfg.SurrealDBUser,
		Password:  cfg.SurrealDBPass,
		AuthLevel: cfg.SurrealDBAuthLevel,
	}

	dbClient, err := db.NewClient(ctx, dbCfg, nil, nil)
	if err != nil {
		return nil, err
	}

	if err := dbClient.InitSchema(ctx, cfg.EmbedDimension); err != nil {
		if closeErr := dbClient.Close(ctx); closeErr != nil {
			slog.Warn("failed to close DB during cleanup", "error", closeErr)
		}
		return nil, err
	}

	// Embedder is optional — nil disables AI features
	var embedder *llm.Embedder
	if cfg.EmbedProvider != config.ProviderNone && cfg.EmbedProvider != "" {
		e, err := llm.NewEmbedder(ctx, cfg, nil)
		if err != nil {
			slog.Warn("embedder initialization failed, AI features disabled", "error", err)
		} else {
			embedder = e
		}
	}

	slog.Info("resolver initialized",
		"embed_provider", cfg.EmbedProvider,
		"embed_dimension", cfg.EmbedDimension,
		"semantic_search", embedder != nil,
	)

	return &Resolver{
		db:              dbClient,
		vaultService:    vault.NewService(dbClient),
		documentService: document.NewService(dbClient, embedder),
		searchService:   search.NewService(dbClient, embedder),
		templateService: template.NewService(dbClient),
	}, nil
}

// DBClient returns the underlying DB client for use by middleware.
func (r *Resolver) DBClient() *db.Client {
	return r.db
}

// Close closes all connections.
func (r *Resolver) Close(ctx context.Context) error {
	if r.db != nil {
		return r.db.Close(ctx)
	}
	return nil
}
