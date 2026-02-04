// Package graph provides GraphQL resolvers for Knowhow.
// This file will not be regenerated automatically.
// It serves as dependency injection for your app.
package graph

import (
	"context"
	"log/slog"

	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/llm"
	"github.com/raphaelgruber/memcp-go/internal/metrics"
	"github.com/raphaelgruber/memcp-go/internal/service"
)

// Resolver is the root resolver with all dependencies.
type Resolver struct {
	db            *db.Client
	entityService *service.EntityService
	searchService *service.SearchService
	ingestService *service.IngestService
	jobManager    *service.JobManager
	cfg           config.Config
	metrics       *metrics.Collector
}

// NewResolver creates a new resolver with all dependencies.
func NewResolver(ctx context.Context, cfg config.Config) (*Resolver, error) {
	// Create metrics collector for runtime statistics
	mc := metrics.NewCollector()

	// Connect to database
	dbCfg := db.Config{
		URL:       cfg.SurrealDBURL,
		Namespace: cfg.SurrealDBNamespace,
		Database:  cfg.SurrealDBDatabase,
		Username:  cfg.SurrealDBUser,
		Password:  cfg.SurrealDBPass,
		AuthLevel: cfg.SurrealDBAuthLevel,
	}

	dbClient, err := db.NewClient(ctx, dbCfg, nil, mc)
	if err != nil {
		return nil, err
	}

	// Initialize schema
	if err := dbClient.InitSchema(ctx); err != nil {
		dbClient.Close(ctx)
		return nil, err
	}

	// Initialize LLM components
	embedder, err := llm.NewEmbedder(cfg, mc)
	if err != nil {
		dbClient.Close(ctx)
		return nil, err
	}

	model, err := llm.NewModel(cfg, mc)
	if err != nil {
		dbClient.Close(ctx)
		return nil, err
	}

	ingestService := service.NewIngestService(dbClient, embedder, model)
	jobManager := service.NewJobManager(cfg.IngestConcurrency, dbClient)

	// Resume any incomplete jobs from previous server run
	if err := jobManager.ResumeIncompleteJobs(ctx, ingestService); err != nil {
		// Log warning but don't fail startup
		slog.Warn("failed to resume incomplete jobs", "error", err)
	}

	return &Resolver{
		db:            dbClient,
		entityService: service.NewEntityService(dbClient, embedder, model),
		searchService: service.NewSearchService(dbClient, embedder, model),
		ingestService: ingestService,
		jobManager:    jobManager,
		cfg:           cfg,
		metrics:       mc,
	}, nil
}

// Close closes all connections.
func (r *Resolver) Close(ctx context.Context) error {
	if r.db != nil {
		return r.db.Close(ctx)
	}
	return nil
}

// WipeData deletes all data from the database. Use for testing only.
func (r *Resolver) WipeData(ctx context.Context) error {
	return r.db.WipeData(ctx)
}
