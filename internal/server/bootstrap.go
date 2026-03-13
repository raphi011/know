// Package server provides the application bootstrap — creates DB client,
// embedder, LLM model, all services, and background workers.
package server

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
	"github.com/raphi011/knowhow/internal/asset"
	"github.com/raphi011/knowhow/internal/config"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/event"
	"github.com/raphi011/knowhow/internal/llm"
	"github.com/raphi011/knowhow/internal/memory"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/remote"
	"github.com/raphi011/knowhow/internal/search"
	"github.com/raphi011/knowhow/internal/template"
	"github.com/raphi011/knowhow/internal/vault"
)

// ServerConfig holds the server's effective configuration for display.
type ServerConfig struct {
	SurrealDBURL           string `json:"surrealdbURL"`
	AuthEnabled            bool   `json:"authEnabled"`
	LLMProvider            string `json:"llmProvider"`
	LLMModel               string `json:"llmModel"`
	EmbedProvider          string `json:"embedProvider"`
	EmbedModel             string `json:"embedModel"`
	EmbedDimension         int    `json:"embedDimension"`
	SemanticSearchEnabled  bool   `json:"semanticSearchEnabled"`
	AgentChatEnabled       bool   `json:"agentChatEnabled"`
	WebSearchEnabled       bool   `json:"webSearchEnabled"`
	ChunkThreshold         int    `json:"chunkThreshold"`
	ChunkTargetSize        int    `json:"chunkTargetSize"`
	ChunkMaxSize           int    `json:"chunkMaxSize"`
	VersionCoalesceMinutes int    `json:"versionCoalesceMinutes"`
	VersionRetentionCount  int    `json:"versionRetentionCount"`
}

// App holds all application services and dependencies.
// mu protects serverConfig, workerCancel, and workerDone.
type App struct {
	mu                       sync.RWMutex
	db                       *db.Client
	vaultService             *vault.Service
	documentService          *document.Service
	assetService             *asset.Service
	searchService            *search.Service
	templateService          *template.Service
	agentService             *agent.Service
	agentRunner              *agent.Runner
	remoteService            *remote.Service
	memoryService            *memory.Service
	bus                      *event.Bus
	workerCancel             context.CancelFunc // guarded by mu
	workerDone               chan struct{}      // guarded by mu
	processingWorkerCancel   context.CancelFunc // guarded by mu
	processingWorkerDone     chan struct{}      // guarded by mu
	processingWorkerInterval time.Duration
	processingWorkerBatch    int
	serverConfig             ServerConfig // guarded by mu
}

// New creates a new App with all dependencies initialized.
func New(ctx context.Context, cfg config.Config) (*App, error) {
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
			slog.Error("auto-bootstrap failed, no-auth mode may not work correctly — check DB connectivity", "error", err)
		}
	}

	// Register eino callback handler for structured LLM/embedding observability.
	// Must be called before creating any models — eino fires callbacks from
	// within Generate/Stream calls for providers that support it.
	llm.RegisterCallbacks()

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

	chunkConfig := cfg.EffectiveChunkConfig()
	if err := chunkConfig.Validate(); err != nil {
		if closeErr := dbClient.Close(ctx); closeErr != nil {
			slog.Warn("failed to close DB during cleanup", "error", closeErr)
		}
		return nil, fmt.Errorf("invalid chunk configuration: %w", err)
	}

	slog.Info("app initialized",
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
	docService := document.NewService(dbClient, embedder, chunkConfig, versionConfig, bus, cfg.EmbedMaxInputChars)

	// workerDone defaults to a closed channel so <-workerDone is a no-op in Close
	embeddingWorkerDone := make(chan struct{})
	close(embeddingWorkerDone)
	processingWorkerDone := make(chan struct{})
	close(processingWorkerDone)

	searchSvc := search.NewService(dbClient, embedder)
	agentSvc := agent.NewService(dbClient, model, searchSvc, docService, cfg.TavilyAPIKey)
	agentRunner := agent.NewRunner(agentSvc, dbClient)

	assetSvc := asset.NewService(dbClient, bus)

	remoteSvc := remote.NewService(dbClient)
	memorySvc := memory.NewService(dbClient, docService, model)

	app := &App{
		db:                       dbClient,
		vaultService:             vault.NewService(dbClient),
		remoteService:            remoteSvc,
		memoryService:            memorySvc,
		assetService:             assetSvc,
		processingWorkerInterval: time.Duration(cfg.ProcessingWorkerInterval) * time.Second,
		processingWorkerBatch:    cfg.ProcessingWorkerBatch,
		documentService:          docService,
		searchService:            searchSvc,
		templateService:          template.NewService(dbClient),
		agentService:             agentSvc,
		agentRunner:              agentRunner,
		bus:                      bus,
		workerDone:               embeddingWorkerDone,
		processingWorkerDone:     processingWorkerDone,
		serverConfig: ServerConfig{
			SurrealDBURL:           cfg.SurrealDBURL,
			AuthEnabled:            !cfg.NoAuth,
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
	app.syncEmbeddingWorker(cfg, embedder)

	// Start background document processing worker (always runs)
	app.startProcessingWorker()

	return app, nil
}

// DBClient returns the underlying DB client.
func (a *App) DBClient() *db.Client {
	return a.db
}

// AgentService returns the agent service.
func (a *App) AgentService() *agent.Service {
	return a.agentService
}

// AgentRunner returns the agent runner for background agent goroutines.
func (a *App) AgentRunner() *agent.Runner {
	return a.agentRunner
}

// EventBus returns the event bus.
func (a *App) EventBus() *event.Bus {
	return a.bus
}

// SearchService returns the search service.
func (a *App) SearchService() *search.Service {
	return a.searchService
}

// DocumentService returns the document service.
func (a *App) DocumentService() *document.Service {
	return a.documentService
}

// VaultService returns the vault service.
func (a *App) VaultService() *vault.Service {
	return a.vaultService
}

// TemplateService returns the template service.
func (a *App) TemplateService() *template.Service {
	return a.templateService
}

// AssetService returns the asset service.
func (a *App) AssetService() *asset.Service {
	return a.assetService
}

// RemoteService returns the remote federation service.
func (a *App) RemoteService() *remote.Service {
	return a.remoteService
}

// MemoryService returns the memory service.
func (a *App) MemoryService() *memory.Service {
	return a.memoryService
}

// NewForTest creates a minimal App for integration tests — no background workers,
// no embedder, no LLM. Only the services needed for handler tests are wired up.
func NewForTest(dbClient *db.Client, docSvc *document.Service, assetSvc *asset.Service, vaultSvc *vault.Service) *App {
	return &App{
		db:              dbClient,
		documentService: docSvc,
		assetService:    assetSvc,
		vaultService:    vaultSvc,
	}
}

// Config returns the server configuration.
func (a *App) Config() ServerConfig {
	return a.serverConfig
}

// Close stops background workers and closes all connections.
func (a *App) Close(ctx context.Context) error {
	a.mu.Lock()
	embedCancel := a.workerCancel
	embedDone := a.workerDone
	a.workerCancel = nil
	procCancel := a.processingWorkerCancel
	procDone := a.processingWorkerDone
	a.processingWorkerCancel = nil
	a.mu.Unlock()

	if embedCancel != nil {
		embedCancel()
		<-embedDone
	}
	if procCancel != nil {
		procCancel()
		<-procDone
	}
	if a.agentRunner != nil {
		a.agentRunner.Shutdown()
	}
	if a.db != nil {
		return a.db.Close(ctx)
	}
	return nil
}

// ReloadLLM re-reads .env, recreates LLM/embedding clients, and swaps them
// into the running services. Called on SIGHUP. On failure, existing working
// providers are kept — only successful initializations are swapped in.
func (a *App) ReloadLLM() error {
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

	// Update chunk config and embed limit (these always reload)
	chunkConfig := cfg.EffectiveChunkConfig()
	if err := chunkConfig.Validate(); err != nil {
		errs = append(errs, fmt.Sprintf("chunk config: %v", err))
	} else {
		a.documentService.SetChunkConfig(chunkConfig, cfg.EmbedMaxInputChars)
	}

	// Only swap providers that were successfully (re)created
	if embedderChanged {
		a.documentService.SetEmbedder(newEmbedder)
		a.searchService.SetEmbedder(newEmbedder)
	}
	if modelChanged {
		a.agentService.SetModel(newModel)
	}

	// Manage embedding worker lifecycle
	if embedderChanged {
		a.syncEmbeddingWorker(cfg, newEmbedder)
	}

	// Update server config
	a.mu.Lock()
	if embedderChanged {
		a.serverConfig.SemanticSearchEnabled = newEmbedder != nil
		a.serverConfig.EmbedProvider = string(cfg.EmbedProvider)
		a.serverConfig.EmbedModel = cfg.EmbedModel
		a.serverConfig.EmbedDimension = cfg.EmbedDimension
	}
	if modelChanged {
		a.serverConfig.AgentChatEnabled = newModel != nil
		a.serverConfig.LLMProvider = string(cfg.LLMProvider)
		a.serverConfig.LLMModel = cfg.LLMModel
	}
	a.serverConfig.WebSearchEnabled = cfg.TavilyAPIKey != ""
	a.serverConfig.ChunkThreshold = chunkConfig.Threshold
	a.serverConfig.ChunkTargetSize = chunkConfig.TargetSize
	a.serverConfig.ChunkMaxSize = chunkConfig.MaxSize
	a.mu.Unlock()

	slog.Info("LLM providers reloaded",
		"semantic_search_changed", embedderChanged,
		"semantic_search", a.searchService.HasSemanticSearch(),
		"agent_chat_changed", modelChanged,
		"agent_chat", a.agentService.Available(),
	)

	if len(errs) > 0 {
		return fmt.Errorf("partial reload failure: %s", strings.Join(errs, "; "))
	}
	return nil
}

// syncEmbeddingWorker starts or stops the background embedding worker based on
// whether an embedder is available. Protects worker fields with a.mu.
func (a *App) syncEmbeddingWorker(cfg config.Config, embedder *llm.Embedder) {
	a.mu.Lock()

	if embedder == nil {
		// Stop worker if running
		if a.workerCancel != nil {
			cancel := a.workerCancel
			done := a.workerDone
			a.workerCancel = nil
			a.mu.Unlock()
			cancel()
			<-done
			slog.Info("stopped embedding worker (embedder removed)")
			return
		}
		a.mu.Unlock()
		return
	}

	// Already running — keep it (it picks up the new embedder via getEmbedder)
	if a.workerCancel != nil {
		a.mu.Unlock()
		return
	}

	// Start new worker
	workerCtx, workerCancel := context.WithCancel(context.Background())
	a.workerCancel = workerCancel
	done := make(chan struct{})
	a.workerDone = done
	a.mu.Unlock()

	interval := time.Duration(cfg.EmbedWorkerInterval) * time.Second
	worker := document.NewEmbeddingWorker(a.documentService, interval, cfg.EmbedWorkerBatch)
	go func() {
		defer close(done)
		worker.Run(workerCtx)
	}()
}

// startProcessingWorker starts the background document processing worker.
// Stops any existing worker first to prevent orphaned goroutines.
func (a *App) startProcessingWorker() {
	a.mu.Lock()

	// Stop existing worker if running
	if a.processingWorkerCancel != nil {
		cancel := a.processingWorkerCancel
		done := a.processingWorkerDone
		a.processingWorkerCancel = nil
		a.mu.Unlock()
		cancel()
		<-done
		a.mu.Lock()
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	a.processingWorkerCancel = workerCancel
	done := make(chan struct{})
	a.processingWorkerDone = done
	a.mu.Unlock()

	worker := document.NewProcessingWorker(a.documentService, a.processingWorkerInterval, a.processingWorkerBatch)
	go func() {
		defer close(done)
		worker.Run(workerCtx)
	}()
}

// seedIfEmpty checks each bootstrap resource (user, vault, membership)
// independently and creates any that are missing. This allows the server to
// self-bootstrap against an empty database (e.g. when running as a launchd
// service) and to recover from a partial previous bootstrap.
func seedIfEmpty(ctx context.Context, dbClient *db.Client) error {
	// 1. Ensure admin user exists
	user, err := dbClient.GetUser(ctx, "admin")
	if err != nil {
		return fmt.Errorf("check admin user: %w", err)
	}
	if user == nil {
		slog.Info("auto-bootstrap: creating admin user")
		user, err = dbClient.CreateUserWithID(ctx, "admin", models.UserInput{
			Name: "admin",
		})
		if err != nil {
			return fmt.Errorf("create admin user: %w", err)
		}
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		return fmt.Errorf("extract user id: %w", err)
	}

	if !user.IsSystemAdmin {
		if err := dbClient.UpdateUserSystemAdmin(ctx, userID, true); err != nil {
			return fmt.Errorf("set system admin: %w", err)
		}
	}

	// 2. Ensure default vault exists
	v, err := dbClient.GetVault(ctx, "default")
	if err != nil {
		return fmt.Errorf("check default vault: %w", err)
	}
	if v == nil {
		slog.Info("auto-bootstrap: creating default vault")
		desc := "Default vault"
		v, err = dbClient.CreateVaultWithID(ctx, "default", userID, models.VaultInput{
			Name:        "default",
			Description: &desc,
		})
		if err != nil {
			return fmt.Errorf("create default vault: %w", err)
		}
	}

	vaultID, err := models.RecordIDString(v.ID)
	if err != nil {
		return fmt.Errorf("extract vault id: %w", err)
	}

	// 3. Ensure vault membership exists
	members, err := dbClient.GetVaultMembers(ctx, vaultID)
	if err != nil {
		return fmt.Errorf("check vault members: %w", err)
	}
	hasMembership := false
	for _, m := range members {
		mid, midErr := models.RecordIDString(m.User)
		if midErr == nil && mid == userID {
			hasMembership = true
			break
		}
	}
	if !hasMembership {
		slog.Info("auto-bootstrap: creating vault membership")
		if _, err := dbClient.CreateVaultMember(ctx, userID, vaultID, models.RoleAdmin); err != nil {
			return fmt.Errorf("create vault member: %w", err)
		}
	}

	slog.Info("auto-bootstrap complete", "user", userID, "vault", vaultID)
	return nil
}
