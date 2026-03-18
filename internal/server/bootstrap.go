// Package server provides the application bootstrap — creates DB client,
// embedder, LLM model, all services, and background workers.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/cloudwego/eino/components/tool"
	"github.com/joho/godotenv"
	"github.com/raphi011/know/internal/agent"
	"github.com/raphi011/know/internal/apify"
	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/config"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/llm"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/memory"
	"github.com/raphi011/know/internal/metrics"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/pipeline"
	"github.com/raphi011/know/internal/remote"
	"github.com/raphi011/know/internal/search"
	"github.com/raphi011/know/internal/stt"
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
	STTProvider            string `json:"sttProvider"`
	STTModel               string `json:"sttModel"`
	TranscriptionEnabled   bool   `json:"transcriptionEnabled"`
	FFmpegInstalled        bool   `json:"ffmpegInstalled"`

	// PDF pipeline
	PopplerInstalled        bool   `json:"popplerInstalled"`
	PDFIngestionEnabled     bool   `json:"pdfIngestionEnabled"`
	MultimodalEmbedProvider string `json:"multimodalEmbedProvider"`
	MultimodalEmbedModel    string `json:"multimodalEmbedModel"`
	MultimodalEmbedEnabled  bool   `json:"multimodalEmbedEnabled"`
	TextExtractorModel      string `json:"textExtractorModel"`
	TextExtractorEnabled    bool   `json:"textExtractorEnabled"`
}

// App holds all application services and dependencies.
// mu protects serverConfig, pipelineWorkerCancel, and pipelineWorkerDone.
type App struct {
	mu           sync.RWMutex
	db           *db.Client
	blobStore    blob.Store
	vaultService *vault.Service
	fileService  *file.Service
	metrics      *metrics.Metrics

	searchService        *search.Service
	agentService         *agent.Service
	agentRunner          *agent.Runner
	remoteService        *remote.Service
	memoryService        *memory.Service
	apifyClient          *apify.Client
	bus                  *event.Bus
	pipelineWorkerCancel context.CancelFunc // guarded by mu
	pipelineWorkerDone   chan struct{}      // guarded by mu
	serverConfig         ServerConfig       // guarded by mu
}

// New creates a new App with all dependencies initialized.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	// Create metrics collector (always created; exposed only if metrics port is configured)
	m := metrics.NewMetrics()

	dbCfg := db.Config{
		URL:       cfg.SurrealDBURL,
		Namespace: cfg.SurrealDBNamespace,
		Database:  cfg.SurrealDBDatabase,
		Username:  cfg.SurrealDBUser,
		Password:  cfg.SurrealDBPass,
		AuthLevel: cfg.SurrealDBAuthLevel,
	}

	dbClient, err := db.NewClient(ctx, dbCfg, nil, m)
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

	blobStore, err := createBlobStore(cfg)
	if err != nil {
		if closeErr := dbClient.Close(ctx); closeErr != nil {
			slog.Warn("failed to close DB during cleanup", "error", closeErr)
		}
		return nil, fmt.Errorf("create blob store: %w", err)
	}
	slog.Info("blob store configured", "type", cfg.BlobStore)

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
		e, err := llm.NewEmbedder(ctx, cfg, m)
		if err != nil {
			slog.Warn("embedder initialization failed, AI features disabled", "error", err)
		} else {
			embedder = e
		}
	}

	// LLM model is optional — nil disables agent chat
	var model *llm.Model
	if cfg.LLMProvider.Enabled() {
		lm, err := llm.NewModel(ctx, cfg, m)
		if err != nil {
			slog.Warn("LLM model initialization failed, agent chat disabled", "error", err)
		} else {
			model = lm
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
	fileSvc := file.NewService(dbClient, blobStore, embedder, chunkConfig, versionConfig, bus, cfg.EmbedMaxInputChars)
	fileSvc.SetAudioSegmentSeconds(cfg.AudioSegmentSeconds)
	fileSvc.SetPDFRenderDPI(cfg.PDFRenderDPI)

	// STT transcriber is optional — nil disables transcription
	var transcriber stt.Transcriber
	if cfg.STTProvider.Enabled() {
		t, err := stt.NewTranscriber(string(cfg.STTProvider), cfg.STTModel, cfg.OpenAIAPIKey, cfg.STTBaseURL)
		if err != nil {
			slog.Warn("STT transcriber initialization failed, transcription disabled", "error", err)
		} else {
			transcriber = t
		}
	}
	if transcriber != nil {
		fileSvc.SetTranscriber(&transcriber)
	}
	if model != nil {
		fileSvc.SetModel(model)
	}

	// Multimodal embedder is optional — nil disables native image/PDF embedding.
	// Falls back to text embedding of extracted content.
	if cfg.MultimodalEmbedProvider.Enabled() {
		me, err := llm.NewGeminiMultimodalEmbedder(ctx, cfg.GoogleAIAPIKey, cfg.MultimodalEmbedModel, cfg.EmbedDimension)
		if err != nil {
			slog.Warn("multimodal embedder initialization failed, multimodal embedding disabled", "error", err)
		} else {
			var iface llm.MultimodalEmbedder = me
			fileSvc.SetMultimodalEmbedder(&iface)
		}
	}

	// Text extractor is optional — nil disables LLM-based text extraction from images/PDFs.
	// Falls back to pdftotext for PDF processing.
	var textExtractorOK bool
	if cfg.GoogleAIAPIKey != "" && cfg.TextExtractorModel != "" {
		te, err := llm.NewGeminiTextExtractor(ctx, cfg.GoogleAIAPIKey, cfg.TextExtractorModel)
		if err != nil {
			slog.Warn("text extractor initialization failed, PDF text extraction will use pdftotext fallback", "error", err)
		} else {
			var iface llm.TextExtractor = te
			fileSvc.SetTextExtractor(&iface)
			textExtractorOK = true
		}
	}

	popplerOK := pipeline.CheckPoppler() == nil

	// pipelineWorkerDone defaults to a closed channel so <-pipelineWorkerDone is a no-op in Close
	pipelineWorkerDone := make(chan struct{})
	close(pipelineWorkerDone)

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
		db:                 dbClient,
		blobStore:          blobStore,
		vaultService:       vaultSvc,
		remoteService:      remoteSvc,
		memoryService:      memorySvc,
		fileService:        fileSvc,
		metrics:            m,
		searchService:      searchSvc,
		agentService:       agentSvc,
		agentRunner:        agentRunner,
		apifyClient:        apifyClient,
		bus:                bus,
		pipelineWorkerDone: pipelineWorkerDone,
		serverConfig: ServerConfig{
			Version:                 cfg.Version,
			Commit:                  cfg.Commit,
			SurrealDBURL:            cfg.SurrealDBURL,
			AuthEnabled:             !cfg.NoAuth,
			LLMProvider:             string(cfg.LLMProvider),
			LLMModel:                cfg.LLMModel,
			EmbedProvider:           string(cfg.EmbedProvider),
			EmbedModel:              cfg.EmbedModel,
			EmbedDimension:          cfg.EmbedDimension,
			SemanticSearchEnabled:   embedder != nil,
			AgentChatEnabled:        model != nil,
			WebSearchEnabled:        cfg.TavilyAPIKey != "",
			ChunkThreshold:          chunkConfig.Threshold,
			ChunkTargetSize:         chunkConfig.TargetSize,
			ChunkMaxSize:            chunkConfig.MaxSize,
			VersionCoalesceMinutes:  cfg.VersionCoalesceMinutes,
			VersionRetentionCount:   cfg.VersionRetentionCount,
			STTProvider:             string(cfg.STTProvider),
			STTModel:                cfg.STTModel,
			TranscriptionEnabled:    transcriber != nil,
			FFmpegInstalled:         isCommandAvailable("ffmpeg"),
			PopplerInstalled:        popplerOK,
			PDFIngestionEnabled:     popplerOK,
			MultimodalEmbedProvider: string(cfg.MultimodalEmbedProvider),
			MultimodalEmbedModel:    cfg.MultimodalEmbedModel,
			MultimodalEmbedEnabled:  cfg.MultimodalEmbedProvider.Enabled(),
			TextExtractorModel:      cfg.TextExtractorModel,
			TextExtractorEnabled:    textExtractorOK,
		},
	}

	// Reconcile conversations stuck in "running" from a previous unclean shutdown
	if n, err := dbClient.ReconcileStaleRunningConversations(ctx); err != nil {
		slog.Warn("failed to reconcile stale running conversations", "error", err)
	} else if n > 0 {
		slog.Info("reconciled stale running conversations", "count", n)
	}

	// Reconcile pipeline jobs stuck in "running" from a previous unclean shutdown
	if n, err := dbClient.ReconcileStaleRunningJobs(ctx); err != nil {
		slog.Warn("failed to reconcile stale running pipeline jobs", "error", err)
	} else if n > 0 {
		slog.Info("reconciled stale running pipeline jobs", "count", n)
	}

	// Start single background pipeline worker (handles parse, embed, transcribe jobs)
	app.startPipelineWorker(cfg)

	return app, nil
}

// Metrics returns the Prometheus metrics collector.
func (a *App) Metrics() *metrics.Metrics {
	return a.metrics
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

// BlobStore returns the blob store.
func (a *App) BlobStore() blob.Store {
	return a.blobStore
}

// NewForTest creates a minimal App for integration tests — no background workers,
// no embedder, no LLM. Only the services needed for handler tests are wired up.
func NewForTest(dbClient *db.Client, blobStore blob.Store, fileSvc *file.Service, vaultSvc *vault.Service) *App {
	return &App{
		db:           dbClient,
		blobStore:    blobStore,
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
	pipelineCancel := a.pipelineWorkerCancel
	pipelineDone := a.pipelineWorkerDone
	a.pipelineWorkerCancel = nil
	a.mu.Unlock()

	var errs []error

	if pipelineCancel != nil {
		logger.Info("stopping pipeline worker")
		pipelineCancel()
		select {
		case <-pipelineDone:
		case <-ctx.Done():
			logger.Warn("pipeline worker did not stop in time")
			errs = append(errs, fmt.Errorf("pipeline worker: %w", ctx.Err()))
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
		e, err := llm.NewEmbedder(ctx, cfg, a.metrics)
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
		m, err := llm.NewModel(ctx, cfg, a.metrics)
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
		a.fileService.SetModel(newModel)
	}

	// Reload STT transcriber
	sttWanted := cfg.STTProvider.Enabled()
	var newTranscriber stt.Transcriber
	transcriberChanged := false

	if sttWanted {
		t, err := stt.NewTranscriber(string(cfg.STTProvider), cfg.STTModel, cfg.OpenAIAPIKey, cfg.STTBaseURL)
		if err != nil {
			slog.Error("STT transcriber reload failed, keeping existing", "error", err)
			errs = append(errs, fmt.Sprintf("transcriber: %v", err))
		} else {
			newTranscriber = t
			transcriberChanged = true
		}
	} else {
		transcriberChanged = true
		newTranscriber = nil
	}

	if transcriberChanged {
		if newTranscriber != nil {
			a.fileService.SetTranscriber(&newTranscriber)
		} else {
			a.fileService.SetTranscriber(nil)
		}
		a.fileService.SetAudioSegmentSeconds(cfg.AudioSegmentSeconds)
	}

	// PDF render DPI is independent of the transcriber — always reload.
	a.fileService.SetPDFRenderDPI(cfg.PDFRenderDPI)

	// Reload multimodal embedder
	mmEmbedWanted := cfg.MultimodalEmbedProvider.Enabled()
	var newMMEmbedder *llm.MultimodalEmbedder
	mmEmbedChanged := false

	if mmEmbedWanted {
		me, err := llm.NewGeminiMultimodalEmbedder(ctx, cfg.GoogleAIAPIKey, cfg.MultimodalEmbedModel, cfg.EmbedDimension)
		if err != nil {
			slog.Error("multimodal embedder reload failed, keeping existing", "error", err)
			errs = append(errs, fmt.Sprintf("multimodal embedder: %v", err))
		} else {
			var iface llm.MultimodalEmbedder = me
			newMMEmbedder = &iface
			mmEmbedChanged = true
		}
	} else {
		mmEmbedChanged = true
		newMMEmbedder = nil
	}

	if mmEmbedChanged {
		a.fileService.SetMultimodalEmbedder(newMMEmbedder)
	}

	// Reload text extractor
	textExtractorWanted := cfg.GoogleAIAPIKey != "" && cfg.TextExtractorModel != ""
	var newTextExtractor *llm.TextExtractor
	textExtractorChanged := false

	if textExtractorWanted {
		te, err := llm.NewGeminiTextExtractor(ctx, cfg.GoogleAIAPIKey, cfg.TextExtractorModel)
		if err != nil {
			slog.Error("text extractor reload failed, keeping existing", "error", err)
			errs = append(errs, fmt.Sprintf("text extractor: %v", err))
		} else {
			var iface llm.TextExtractor = te
			newTextExtractor = &iface
			textExtractorChanged = true
		}
	} else {
		textExtractorChanged = true
		newTextExtractor = nil
	}

	if textExtractorChanged {
		a.fileService.SetTextExtractor(newTextExtractor)
	}

	popplerOK := pipeline.CheckPoppler() == nil

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
	if transcriberChanged {
		a.serverConfig.TranscriptionEnabled = newTranscriber != nil
		a.serverConfig.STTProvider = string(cfg.STTProvider)
		a.serverConfig.STTModel = cfg.STTModel
	}
	if mmEmbedChanged {
		a.serverConfig.MultimodalEmbedProvider = string(cfg.MultimodalEmbedProvider)
		a.serverConfig.MultimodalEmbedModel = cfg.MultimodalEmbedModel
		a.serverConfig.MultimodalEmbedEnabled = newMMEmbedder != nil
	}
	if textExtractorChanged {
		a.serverConfig.TextExtractorModel = cfg.TextExtractorModel
		a.serverConfig.TextExtractorEnabled = newTextExtractor != nil
	}
	a.serverConfig.FFmpegInstalled = isCommandAvailable("ffmpeg")
	a.serverConfig.PopplerInstalled = popplerOK
	a.serverConfig.PDFIngestionEnabled = popplerOK
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

// startPipelineWorker starts the single background pipeline worker that dispatches
// all job types (parse, embed, transcribe). Always runs; embedder and transcriber
// are picked up dynamically via atomic pointers on the file service.
func (a *App) startPipelineWorker(cfg config.Config) {
	a.mu.Lock()

	// Stop existing worker if running (e.g. called on reload)
	if a.pipelineWorkerCancel != nil {
		cancel := a.pipelineWorkerCancel
		done := a.pipelineWorkerDone
		a.pipelineWorkerCancel = nil
		a.mu.Unlock()
		cancel()
		<-done
		a.mu.Lock()
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	a.pipelineWorkerCancel = workerCancel
	done := make(chan struct{})
	a.pipelineWorkerDone = done
	a.mu.Unlock()

	interval := time.Duration(cfg.PipelineWorkerInterval) * time.Second
	w := pipeline.NewWorker(a.db, a.bus, interval, cfg.PipelineWorkerBatch, a.metrics)
	w.Register("parse", file.ParseHandler(a.fileService, a.bus))
	w.Register("transcribe", file.TranscribeHandler(a.fileService, a.bus))
	w.Register("pdf", file.PDFHandler(a.fileService, a.bus))
	w.Register("summarize", file.SummarizeHandler(a.fileService, a.bus))
	w.Register("embed", file.EmbedHandler(a.fileService))
	go func() {
		defer close(done)
		w.Run(workerCtx)
	}()
}

// createBlobStore creates a blob.Store based on the config.
func createBlobStore(cfg config.Config) (blob.Store, error) {
	switch cfg.BlobStore {
	case "fs":
		if err := os.MkdirAll(cfg.BlobDir, 0o755); err != nil {
			return nil, fmt.Errorf("create blob dir %s: %w", cfg.BlobDir, err)
		}
		return blob.NewFS(cfg.BlobDir), nil
	case "s3":
		if cfg.BlobS3Bucket == "" {
			return nil, fmt.Errorf("KNOW_BLOB_S3_BUCKET is required when blob store is s3")
		}
		awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithRegion(cfg.BlobS3Region),
		)
		if err != nil {
			return nil, fmt.Errorf("load AWS config: %w", err)
		}
		opts := []func(*s3.Options){}
		if cfg.BlobS3Endpoint != "" {
			opts = append(opts, func(o *s3.Options) {
				o.BaseEndpoint = &cfg.BlobS3Endpoint
				o.UsePathStyle = true
			})
		}
		client := s3.NewFromConfig(awsCfg, opts...)
		return blob.NewS3(client, cfg.BlobS3Bucket, cfg.BlobS3Prefix), nil
	default:
		return nil, fmt.Errorf("unknown blob store: %q", cfg.BlobStore)
	}
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

// isCommandAvailable checks if a command is available in PATH.
func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
