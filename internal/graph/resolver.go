// Package graph provides GraphQL resolvers for Knowhow.
// This file will not be regenerated automatically.
package graph

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/raphi011/knowhow/internal/agent"
	"github.com/raphi011/knowhow/internal/config"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/event"
	"github.com/raphi011/knowhow/internal/llm"
	"github.com/raphi011/knowhow/internal/search"
	"github.com/raphi011/knowhow/internal/template"
	"github.com/raphi011/knowhow/internal/vault"
)

// Resolver is the root resolver with all dependencies.
type Resolver struct {
	db              *db.Client
	vaultService    *vault.Service
	documentService *document.Service
	searchService   *search.Service
	templateService *template.Service
	model           *llm.Model
	agentService    *agent.Service
	bus             *event.Bus
	workerCancel    context.CancelFunc
	workerDone      chan struct{}
	serverConfig    ServerConfig
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

	// LLM model is optional — nil disables agent chat
	var model *llm.Model
	if cfg.LLMProvider != config.ProviderNone && cfg.LLMProvider != "" {
		m, err := llm.NewModel(ctx, cfg, nil)
		if err != nil {
			slog.Warn("LLM model initialization failed, agent chat disabled", "error", err)
		} else {
			model = m
		}
	}

	chunkConfig := cfg.ChunkConfig()
	if err := chunkConfig.Validate(); err != nil {
		if closeErr := dbClient.Close(ctx); closeErr != nil {
			slog.Warn("failed to close DB during cleanup", "error", closeErr)
		}
		return nil, fmt.Errorf("invalid chunk configuration: %w", err)
	}

	slog.Info("resolver initialized",
		"embed_provider", cfg.EmbedProvider,
		"embed_dimension", cfg.EmbedDimension,
		"semantic_search", embedder != nil,
		"agent_chat", model != nil,
		"web_search", cfg.TavilyAPIKey != "",
		"chunk_threshold", chunkConfig.Threshold,
		"chunk_target", chunkConfig.TargetSize,
		"chunk_max", chunkConfig.MaxSize,
	)

	versionConfig := document.VersionConfig{
		CoalesceMinutes: cfg.VersionCoalesceMinutes,
		RetentionCount:  cfg.VersionRetentionCount,
	}
	bus := event.New()
	docService := document.NewService(dbClient, embedder, chunkConfig, versionConfig, bus)

	workerDone := make(chan struct{})
	close(workerDone) // safe default: <-workerDone returns immediately if no worker

	searchSvc := search.NewService(dbClient, embedder)
	agentSvc := agent.NewService(dbClient, model, searchSvc, docService, cfg.TavilyAPIKey)

	r := &Resolver{
		db:              dbClient,
		vaultService:    vault.NewService(dbClient),
		documentService: docService,
		searchService:   searchSvc,
		templateService: template.NewService(dbClient),
		model:           model,
		agentService:    agentSvc,
		bus:             bus,
		workerDone:      workerDone,
		serverConfig: ServerConfig{
			LLMProvider:            string(cfg.LLMProvider),
			LLMModel:               cfg.LLMModel,
			EmbedProvider:          string(cfg.EmbedProvider),
			EmbedModel:             cfg.EmbedModel,
			EmbedDimension:         cfg.EmbedDimension,
			SemanticSearchEnabled:  embedder != nil,
			AgentChatEnabled:       model != nil,
			WebSearchEnabled:       cfg.TavilyAPIKey != "",
			ChunkThreshold:         chunkConfig.Threshold,
			ChunkTargetSize:        chunkConfig.TargetSize,
			ChunkMaxSize:           chunkConfig.MaxSize,
			VersionCoalesceMinutes: cfg.VersionCoalesceMinutes,
			VersionRetentionCount:  cfg.VersionRetentionCount,
		},
	}

	// Start background embedding worker if embedder is available
	if embedder != nil {
		workerCtx, workerCancel := context.WithCancel(context.Background())
		r.workerCancel = workerCancel
		r.workerDone = make(chan struct{})
		interval := time.Duration(cfg.EmbedWorkerInterval) * time.Second
		worker := document.NewEmbeddingWorker(docService, interval, cfg.EmbedWorkerBatch)
		go func() {
			defer close(r.workerDone)
			worker.Run(workerCtx)
		}()
	}

	return r, nil
}

// DBClient returns the underlying DB client for use by middleware.
func (r *Resolver) DBClient() *db.Client {
	return r.db
}

// AgentService returns the agent service for use by the SSE handler.
func (r *Resolver) AgentService() *agent.Service {
	return r.agentService
}

// EventBus returns the event bus for use by the SSE handler.
func (r *Resolver) EventBus() *event.Bus {
	return r.bus
}

// SearchService returns the search service.
func (r *Resolver) SearchService() *search.Service {
	return r.searchService
}

// DocumentService returns the document service.
func (r *Resolver) DocumentService() *document.Service {
	return r.documentService
}

// VaultService returns the vault service.
func (r *Resolver) VaultService() *vault.Service {
	return r.vaultService
}

// Close stops background workers and closes all connections.
func (r *Resolver) Close(ctx context.Context) error {
	if r.workerCancel != nil {
		r.workerCancel()
		<-r.workerDone
	}
	if r.db != nil {
		return r.db.Close(ctx)
	}
	return nil
}
