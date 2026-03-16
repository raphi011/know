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

	"github.com/cloudwego/eino/components/tool"
	"github.com/joho/godotenv"
	"github.com/raphi011/know/internal/agent"
	"github.com/raphi011/know/internal/apify"
	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/config"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/memory"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/remote"
	"github.com/raphi011/know/internal/search"
	"github.com/raphi011/know/internal/tools"
	"github.com/raphi011/know/internal/vault"
)

// ServerConfig holds the server's effective configuration for display.
type ServerConfig struct {
	Version                string `json:"version"`
	Commit                 string `json:"commit"`
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
	mu           sync.RWMutex
	db           *db.Client
	vaultService *vault.Service
	fileService  *file.Service

	searchService            *search.Service
	agentService             *agent.Service
	agentRunner              *agent.Runner
	remoteService            *remote.Service
	memoryService            *memory.Service
	apifyClient              *apify.Client
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
	// Global handlers apply to all subsequent Generate/Stream calls.
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

	versionConfig := file.VersionConfig{
		CoalesceMinutes: cfg.VersionCoalesceMinutes,
		RetentionCount:  cfg.VersionRetentionCount,
	}
	bus := event.New()
	fileSvc := file.NewService(dbClient, embedder, chunkConfig, versionConfig, bus, cfg.EmbedMaxInputChars)

	// workerDone defaults to a closed channel so <-workerDone is a no-op in Close
	embeddingWorkerDone := make(chan struct{})
	close(embeddingWorkerDone)
	processingWorkerDone := make(chan struct{})
	close(processingWorkerDone)

	searchSvc := search.NewService(dbClient, embedder)
	remoteSvc := remote.NewService(dbClient)

	// Build multi-vault tool resolvers for the agent.
	localExecutor := &tools.Executor{
		DB:      dbClient,
		Search:  searchSvc,
		FileSvc: fileSvc,
	}
	vaultSvc := vault.NewService(dbClient)
	agentTools := buildAgentTools(localExecutor, vaultSvc, remoteSvc)
	var apifyClient *apify.Client
	if cfg.ApifyToken != "" {
		apifyClient = apify.New(cfg.ApifyToken)
	}
	agentSvc := agent.NewService(dbClient, model, agentTools, cfg.TavilyAPIKey, apifyClient)
	agentRunner := agent.NewRunner(agentSvc, dbClient)
	memorySvc := memory.NewService(dbClient, fileSvc, model)

	app := &App{
		db:                       dbClient,
		vaultService:             vaultSvc,
		remoteService:            remoteSvc,
		memoryService:            memorySvc,
		processingWorkerInterval: time.Duration(cfg.ProcessingWorkerInterval) * time.Second,
		processingWorkerBatch:    cfg.ProcessingWorkerBatch,
		fileService:              fileSvc,
		searchService:            searchSvc,
		agentService:             agentSvc,
		agentRunner:              agentRunner,
		apifyClient:              apifyClient,
		bus:                      bus,
		workerDone:               embeddingWorkerDone,
		processingWorkerDone:     processingWorkerDone,
		serverConfig: ServerConfig{
			Version:                cfg.Version,
			Commit:                 cfg.Commit,
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

	// Reconcile conversations stuck in "running" from a previous unclean shutdown
	if n, err := dbClient.ReconcileStaleRunningConversations(ctx); err != nil {
		slog.Warn("failed to reconcile stale running conversations", "error", err)
	} else if n > 0 {
		slog.Info("reconciled stale running conversations", "count", n)
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

// FileService returns the file service.
func (a *App) FileService() *file.Service {
	return a.fileService
}

// VaultService returns the vault service.
func (a *App) VaultService() *vault.Service {
	return a.vaultService
}

// RemoteService returns the remote federation service.
func (a *App) RemoteService() *remote.Service {
	return a.remoteService
}

// MemoryService returns the memory service.
func (a *App) MemoryService() *memory.Service {
	return a.memoryService
}

// ApifyClient returns the Apify client, or nil if not configured.
func (a *App) ApifyClient() *apify.Client {
	return a.apifyClient
}

// NewForTest creates a minimal App for integration tests — no background workers,
// no embedder, no LLM. Only the services needed for handler tests are wired up.
func NewForTest(dbClient *db.Client, fileSvc *file.Service, vaultSvc *vault.Service) *App {
	return &App{
		db:           dbClient,
		fileService:  fileSvc,
		vaultService: vaultSvc,
	}
}

// Config returns the server configuration.
func (a *App) Config() ServerConfig {
	return a.serverConfig
}

// Close stops background workers and closes all connections.
// The context deadline is respected — if it expires, Close continues with
// best-effort cleanup but includes the deadline error in the returned error.
func (a *App) Close(ctx context.Context) error {
	logger := logutil.FromCtx(ctx)

	a.mu.Lock()
	embedCancel := a.workerCancel
	embedDone := a.workerDone
	a.workerCancel = nil
	procCancel := a.processingWorkerCancel
	procDone := a.processingWorkerDone
	a.processingWorkerCancel = nil
	a.mu.Unlock()

	var errs []error

	if embedCancel != nil {
		logger.Info("stopping embedding worker")
		embedCancel()
		select {
		case <-embedDone:
		case <-ctx.Done():
			logger.Warn("embedding worker did not stop in time")
			errs = append(errs, fmt.Errorf("embedding worker: %w", ctx.Err()))
		}
	}
	if procCancel != nil {
		logger.Info("stopping processing worker")
		procCancel()
		select {
		case <-procDone:
		case <-ctx.Done():
			logger.Warn("processing worker did not stop in time")
			errs = append(errs, fmt.Errorf("processing worker: %w", ctx.Err()))
		}
	}
	if a.agentRunner != nil {
		if err := a.agentRunner.Shutdown(ctx); err != nil {
			logger.Warn("agent runner shutdown incomplete", "error", err)
			errs = append(errs, fmt.Errorf("agent runner: %w", err))
		}
	}
	if a.bus != nil {
		logger.Info("closing event bus")
		a.bus.Close()
	}
	if a.db != nil {
		logger.Info("closing database connection")
		// Use a fresh timeout for DB close — it's the most important cleanup
		// step and the parent ctx may already be expired.
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer dbCancel()
		if err := a.db.Close(dbCtx); err != nil {
			errs = append(errs, fmt.Errorf("db close: %w", err))
		}
	}
	return errors.Join(errs...)
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
		a.fileService.SetChunkConfig(chunkConfig, cfg.EmbedMaxInputChars)
	}

	// Only swap providers that were successfully (re)created
	if embedderChanged {
		a.fileService.SetEmbedder(newEmbedder)
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
	worker := file.NewEmbeddingWorker(a.fileService, interval, cfg.EmbedWorkerBatch, a.bus)
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

	worker := file.NewProcessingWorker(a.fileService, a.processingWorkerInterval, a.processingWorkerBatch, a.bus)
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

// buildAgentTools creates multi-vault tool wrappers for the agent service.
// The resolvers close over the local executor, vault service, and remote service
// to route tool calls to the appropriate vault.
func buildAgentTools(localExecutor *tools.Executor, vaultSvc *vault.Service, remoteSvc *remote.Service) []tool.BaseTool {
	resolver := func(ctx context.Context) ([]tools.VaultRef, error) {
		localIDs, err := resolveVaultIDs(ctx, vaultSvc)
		if err != nil {
			return nil, err
		}

		refs := make([]tools.VaultRef, 0, len(localIDs))
		for _, id := range localIDs {
			refs = append(refs, tools.VaultRef{
				VaultID:  id,
				Executor: localExecutor,
			})
		}

		if remoteSvc == nil {
			return refs, nil
		}

		remoteVaults, err := remoteSvc.ListRemoteVaults(ctx)
		if err != nil {
			logutil.FromCtx(ctx).Warn("failed to list remote vaults, using local only", "error", err)
			return refs, nil
		}
		for _, rv := range remoteVaults {
			client, clientErr := remoteSvc.ClientFor(ctx, rv.RemoteName)
			if clientErr != nil {
				logutil.FromCtx(ctx).Warn("failed to get client for remote, skipping", "remote", rv.RemoteName, "error", clientErr)
				continue
			}
			refs = append(refs, tools.VaultRef{
				VaultID:   rv.VaultID,
				Executor:  remote.NewExecutor(client, rv.RemoteName),
				Namespace: rv.Namespace,
			})
		}

		return refs, nil
	}

	writeResolver := func(ctx context.Context, vaultName string) (tools.VaultRef, error) {
		if vaultName != "" && strings.Contains(vaultName, "/") {
			parts := strings.SplitN(vaultName, "/", 2)
			remoteName := parts[0]

			if remoteSvc == nil {
				return tools.VaultRef{}, fmt.Errorf("remote vaults not configured")
			}

			client, err := remoteSvc.ClientFor(ctx, remoteName)
			if err != nil {
				return tools.VaultRef{}, fmt.Errorf("resolve remote %q: %w", remoteName, err)
			}

			remoteVaults, err := remoteSvc.ListRemoteVaults(ctx)
			if err != nil {
				return tools.VaultRef{}, fmt.Errorf("list remote vaults: %w", err)
			}
			for _, rv := range remoteVaults {
				if rv.Namespace == vaultName {
					return tools.VaultRef{
						VaultID:   rv.VaultID,
						Executor:  remote.NewExecutor(client, remoteName),
						Namespace: rv.Namespace,
					}, nil
				}
			}
			return tools.VaultRef{}, fmt.Errorf("remote vault %q not found", vaultName)
		}

		vaultIDs, err := resolveVaultIDs(ctx, vaultSvc)
		if err != nil {
			return tools.VaultRef{}, err
		}
		if len(vaultIDs) == 0 {
			return tools.VaultRef{}, fmt.Errorf("no vaults accessible")
		}

		return tools.VaultRef{
			VaultID:  vaultIDs[0],
			Executor: localExecutor,
		}, nil
	}

	return tools.NewMultiVaultTools(resolver, writeResolver)
}

// resolveVaultIDs returns the list of vault IDs the caller has access to.
// In no-auth mode (wildcard access), it fetches all vault IDs from the DB.
func resolveVaultIDs(ctx context.Context, vaultService *vault.Service) ([]string, error) {
	ac, err := auth.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve vault IDs: %w", err)
	}

	hasWildcard := false
	for _, vp := range ac.Vaults {
		if vp.VaultID == auth.WildcardVaultAccess {
			hasWildcard = true
			break
		}
	}

	if !hasWildcard {
		ids := make([]string, 0, len(ac.Vaults))
		for _, vp := range ac.Vaults {
			ids = append(ids, vp.VaultID)
		}
		return ids, nil
	}

	vaults, err := vaultService.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}
	ids := make([]string, 0, len(vaults))
	for _, v := range vaults {
		id, idErr := models.RecordIDString(v.ID)
		if idErr != nil {
			logutil.FromCtx(ctx).Warn("failed to extract vault ID, skipping", "vault_name", v.Name, "error", idErr)
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}
