// Package graph provides GraphQL resolvers for Knowhow.
// This file will not be regenerated automatically.
package graph

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
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
// mu protects serverConfig, workerCancel, and workerDone.
type Resolver struct {
	mu              sync.RWMutex
	db              *db.Client
	vaultService    *vault.Service
	documentService *document.Service
	searchService   *search.Service
	templateService *template.Service
	agentService    *agent.Service
	bus             *event.Bus
	workerCancel    context.CancelFunc // guarded by mu
	workerDone      chan struct{}       // guarded by mu
	serverConfig    ServerConfig       // guarded by mu
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

	// In no-auth mode, auto-bootstrap the default user/vault if the DB is empty.
	// This allows the server to self-provision when running as a launchd service.
	if cfg.NoAuth {
		if err := seedIfEmpty(ctx, dbClient); err != nil {
			slog.Warn("auto-bootstrap failed", "error", err)
		}
	}

	slog.Info("LLM config",
		"embed_provider", cfg.EmbedProvider,
		"embed_model", cfg.EmbedModel,
		"embed_dimension", cfg.EmbedDimension,
		"llm_provider", cfg.LLMProvider,
		"llm_model", cfg.LLMModel,
	)

	// Embedder is optional — nil disables AI features
	var embedder *llm.Embedder
	if cfg.EmbedProvider.Enabled() {
		e, err := llm.NewEmbedder(ctx, cfg, nil)
		if err != nil {
			slog.Warn("embedder initialization failed, AI features disabled", "error", err)
		} else {
			embedder = e
		}
	}

	// LLM model is optional — nil disables agent chat
	var model *llm.Model
	if cfg.LLMProvider.Enabled() {
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

	// workerDone defaults to a closed channel so <-workerDone is a no-op in Close
	workerDone := make(chan struct{})
	close(workerDone)

	searchSvc := search.NewService(dbClient, embedder)
	agentSvc := agent.NewService(dbClient, model, searchSvc, docService, cfg.TavilyAPIKey)

	r := &Resolver{
		db:              dbClient,
		vaultService:    vault.NewService(dbClient),
		documentService: docService,
		searchService:   searchSvc,
		templateService: template.NewService(dbClient),
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
	r.syncEmbeddingWorker(cfg, embedder)

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
	r.mu.Lock()
	cancel := r.workerCancel
	done := r.workerDone
	r.workerCancel = nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}
	if r.db != nil {
		return r.db.Close(ctx)
	}
	return nil
}

// ReloadLLM re-reads .env, recreates LLM/embedding clients, and swaps them
// into the running services. Called on SIGHUP. On failure, existing working
// providers are kept — only successful initializations are swapped in.
func (r *Resolver) ReloadLLM() error {
	// Load .env into process environment (overwrite existing vars)
	if err := godotenv.Overload(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Debug("no .env file to load")
		} else {
			slog.Warn("failed to load .env file, using current environment", "error", err)
		}
	}

	cfg := config.Load()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var errs []string

	// Reload embedder — only swap if initialization succeeds
	embedderWanted := cfg.EmbedProvider.Enabled()
	var newEmbedder *llm.Embedder
	embedderChanged := false

	if embedderWanted {
		e, err := llm.NewEmbedder(ctx, cfg, nil)
		if err != nil {
			slog.Error("embedder reload failed, keeping existing", "error", err)
			errs = append(errs, fmt.Sprintf("embedder: %v", err))
		} else {
			newEmbedder = e
			embedderChanged = true
		}
	} else {
		// Explicitly disabled
		embedderChanged = true
		newEmbedder = nil
	}

	// Reload model — only swap if initialization succeeds
	modelWanted := cfg.LLMProvider.Enabled()
	var newModel *llm.Model
	modelChanged := false

	if modelWanted {
		m, err := llm.NewModel(ctx, cfg, nil)
		if err != nil {
			slog.Error("LLM model reload failed, keeping existing", "error", err)
			errs = append(errs, fmt.Sprintf("model: %v", err))
		} else {
			newModel = m
			modelChanged = true
		}
	} else {
		modelChanged = true
		newModel = nil
	}

	// Only swap providers that were successfully (re)created
	if embedderChanged {
		r.documentService.SetEmbedder(newEmbedder)
		r.searchService.SetEmbedder(newEmbedder)
	}
	if modelChanged {
		r.agentService.SetModel(newModel)
	}

	// Manage embedding worker lifecycle
	if embedderChanged {
		r.syncEmbeddingWorker(cfg, newEmbedder)
	}

	// Update server config
	r.mu.Lock()
	if embedderChanged {
		r.serverConfig.SemanticSearchEnabled = newEmbedder != nil
		r.serverConfig.EmbedProvider = string(cfg.EmbedProvider)
		r.serverConfig.EmbedModel = cfg.EmbedModel
		r.serverConfig.EmbedDimension = cfg.EmbedDimension
	}
	if modelChanged {
		r.serverConfig.AgentChatEnabled = newModel != nil
		r.serverConfig.LLMProvider = string(cfg.LLMProvider)
		r.serverConfig.LLMModel = cfg.LLMModel
	}
	r.serverConfig.WebSearchEnabled = cfg.TavilyAPIKey != ""
	r.mu.Unlock()

	slog.Info("LLM providers reloaded (only LLM/embedding settings are applied)",
		"semantic_search_changed", embedderChanged,
		"semantic_search", r.searchService.HasSemanticSearch(),
		"agent_chat_changed", modelChanged,
		"agent_chat", r.agentService.Available(),
	)

	if len(errs) > 0 {
		return fmt.Errorf("partial reload failure: %s", strings.Join(errs, "; "))
	}
	return nil
}

// syncEmbeddingWorker starts or stops the background embedding worker based on
// whether an embedder is available. Protects worker fields with r.mu.
func (r *Resolver) syncEmbeddingWorker(cfg config.Config, embedder *llm.Embedder) {
	r.mu.Lock()

	if embedder == nil {
		// Stop worker if running
		if r.workerCancel != nil {
			cancel := r.workerCancel
			done := r.workerDone
			r.workerCancel = nil
			r.mu.Unlock()
			cancel()
			<-done
			slog.Info("stopped embedding worker (embedder removed)")
			return
		}
		r.mu.Unlock()
		return
	}

	// Already running — keep it (it picks up the new embedder via getEmbedder)
	if r.workerCancel != nil {
		r.mu.Unlock()
		return
	}

	// Start new worker
	workerCtx, workerCancel := context.WithCancel(context.Background())
	r.workerCancel = workerCancel
	done := make(chan struct{})
	r.workerDone = done
	r.mu.Unlock()

	interval := time.Duration(cfg.EmbedWorkerInterval) * time.Second
	worker := document.NewEmbeddingWorker(r.documentService, interval, cfg.EmbedWorkerBatch)
	go func() {
		defer close(done)
		worker.Run(workerCtx)
	}()
}
