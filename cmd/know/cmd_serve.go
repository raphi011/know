package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/raphi011/know/internal/api"
	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/config"
	"github.com/raphi011/know/internal/event"
	"github.com/raphi011/know/internal/mcptools"
	knownfs "github.com/raphi011/know/internal/nfs"
	"github.com/raphi011/know/internal/oidc"
	"github.com/raphi011/know/internal/server"
	"github.com/raphi011/know/internal/sshd"
	"github.com/raphi011/know/internal/tools"
	knowdav "github.com/raphi011/know/internal/webdav"
	"github.com/spf13/cobra"
)

var (
	servePort        int
	serveNoAuth      bool
	serveSSH         bool
	serveSSHPort     int
	serveNFS         bool
	serveNFSPort     int
	serveMetricsPort string
	serveLogLevel    string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Know server",
	Long: `Start the Know HTTP server with REST API, WebDAV, and optional MCP/SSH endpoints.

Configuration is loaded from environment variables (see config package).
Flags override the corresponding env vars.

Examples:
  know serve
  know serve --port 8080
  know serve --no-auth --log-level debug
  know serve --ssh --ssh-port 2222`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", envOrDefaultInt("KNOW_SERVER_PORT", 8484), "HTTP server port")
	serveCmd.Flags().BoolVar(&serveNoAuth, "no-auth", envOrDefaultBool("KNOW_NO_AUTH", false), "disable token authentication")
	serveCmd.Flags().BoolVar(&serveSSH, "ssh", envOrDefaultBool("KNOW_SSH_ENABLED", false), "enable SSH/SFTP server")
	serveCmd.Flags().IntVar(&serveSSHPort, "ssh-port", envOrDefaultInt("KNOW_SSH_PORT", 2222), "SSH server port")
	serveCmd.Flags().BoolVar(&serveNFS, "nfs", envOrDefaultBool("KNOW_NFS_ENABLED", false), "enable NFS server (localhost only)")
	serveCmd.Flags().IntVar(&serveNFSPort, "nfs-port", envOrDefaultInt("KNOW_NFS_PORT", 2049), "NFS server port")
	serveCmd.Flags().StringVar(&serveMetricsPort, "metrics-port", envOrDefault("KNOW_METRICS_PORT", ""), "Prometheus metrics port (default: disabled)")
	serveCmd.Flags().StringVar(&serveLogLevel, "log-level", envOrDefault("KNOW_LOG_LEVEL", "info"), "log level (debug, info, warn, error)")
}

func runServe(_ *cobra.Command, _ []string) error {
	// Initialize logging with dynamic level
	var levelVar slog.LevelVar
	var level slog.Level
	if err := level.UnmarshalText([]byte(serveLogLevel)); err != nil {
		return fmt.Errorf("invalid log level %q: %w", serveLogLevel, err)
	}
	levelVar.Set(level)

	logFile := os.Getenv("KNOW_LOG_FILE")
	logger, logCleanup, err := config.SetupLogger(logFile, &levelVar)
	if err != nil {
		return fmt.Errorf("setup logger: %w", err)
	}
	defer func() {
		if err := logCleanup(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close log file: %v\n", err)
		}
	}()
	slog.SetDefault(logger)

	port := fmt.Sprintf("%d", servePort)

	// Load configuration and apply flag overrides
	cfg := config.Load()
	cfg.NoAuth = serveNoAuth

	// Production mode guards
	if cfg.IsProduction() {
		if serveNoAuth {
			return fmt.Errorf("KNOW_NO_AUTH is not allowed in production mode (KNOW_ENVIRONMENT=production)")
		}
		if cfg.TLSSkipVerify {
			return fmt.Errorf("KNOW_TLS_SKIP_VERIFY=true is not allowed in production mode (KNOW_ENVIRONMENT=production)")
		}
		if serveNFS {
			slog.Warn("NFS server enabled in production mode — NFS has no per-user authentication, ensure network access is restricted")
		}
		slog.Info("running in production mode")
	}

	// In no-auth mode, bind to localhost only to prevent accidental public exposure.
	listenHost := ""
	if serveNoAuth {
		listenHost = "127.0.0.1"
		slog.Warn("no-auth mode: binding to localhost only")
	}

	slog.Info("starting know server", "version", version, "commit", commit, "port", port, "host", listenHost)

	// Bind the port early so we fail fast if another instance is still running.
	ln, err := net.Listen("tcp", listenHost+":"+port)
	if err != nil {
		return fmt.Errorf("bind port %s: %w", port, err)
	}
	cfg.SSHEnabled = serveSSH
	cfg.SSHPort = fmt.Sprintf("%d", serveSSHPort)
	cfg.NFSEnabled = serveNFS
	cfg.NFSPort = fmt.Sprintf("%d", serveNFSPort)
	if serveMetricsPort != "" {
		cfg.MetricsPort = serveMetricsPort
	}
	cfg.Version = version
	cfg.Commit = commit

	// Create application with all dependencies
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	app, err := server.New(ctx, cfg)
	cancel()
	if err != nil {
		return fmt.Errorf("create app: %w", err)
	}

	// serverCtx is cancelled on shutdown to stop background goroutines
	// (e.g. WebDAV pending sweep).
	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel() // safety net for early-return paths; also called explicitly in shutdown

	// Listen for SIGHUP to reload LLM config from .env
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			func() {
				defer func() {
					if p := recover(); p != nil {
						slog.Error("ReloadLLM panicked", "error", p)
					}
				}()
				slog.Info("SIGHUP received, reloading config")
				if err := app.ReloadLLM(); err != nil {
					slog.Warn("LLM reload failed", "error", err)
				}
				// Reload log level from environment
				if newLevel := os.Getenv("KNOW_LOG_LEVEL"); newLevel != "" {
					var parsed slog.Level
					if err := parsed.UnmarshalText([]byte(newLevel)); err != nil {
						slog.Warn("invalid KNOW_LOG_LEVEL after reload", "value", newLevel, "error", err)
					} else if parsed != levelVar.Level() {
						old := levelVar.Level()
						levelVar.Set(parsed)
						slog.Info("log level changed", "old", old, "new", parsed)
					}
				}
			}()
		}
	}()

	// Setup routes
	mux := http.NewServeMux()

	// Validate OIDC config before attempting setup
	if err := cfg.ValidateOIDC(); err != nil {
		return err
	}

	// Auth rate limiter (wraps OIDC routes only)
	var authRateLimiter *api.IPRateLimiter
	if cfg.RateLimitAuthRPS > 0 {
		authRateLimiter = api.NewIPRateLimiter(cfg.RateLimitAuthRPS, cfg.RateLimitAuthBurst, "auth", app.Metrics())
		defer authRateLimiter.Stop()
		slog.Info("auth rate limiting enabled", "rps", cfg.RateLimitAuthRPS, "burst", cfg.RateLimitAuthBurst)
	}

	// OIDC auth routes (unauthenticated — must be registered before auth middleware)
	var oidcHandler *api.AuthHandler
	if cfg.OIDCEnabled {
		var oidcProvider oidc.Provider
		switch cfg.OIDCProviderType {
		case "github":
			oidcProvider = oidc.NewGitHub(cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCRedirectURL, cfg.OIDCProviderName)
		default:
			oidcCtx, oidcCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer oidcCancel()
			p, oidcErr := oidc.NewOIDC(oidcCtx, cfg.OIDCIssuerURL, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCRedirectURL, nil, cfg.OIDCProviderName)
			if oidcErr != nil {
				return fmt.Errorf("init oidc provider: %w", oidcErr)
			}
			oidcProvider = p
		}
		var oidcErr error
		oidcHandler, oidcErr = api.NewAuthHandler(oidcProvider, app.DBClient(), cfg.SelfSignupEnabled, cfg.OIDCRedirectURL, cfg.TokenMaxLifetimeDays)
		if oidcErr != nil {
			return fmt.Errorf("init oidc handler: %w", oidcErr)
		}
		if authRateLimiter != nil {
			oidcHandler.RegisterRoutes(mux, authRateLimiter.Middleware(cfg.TrustXForwardedFor))
		} else {
			oidcHandler.RegisterRoutes(mux)
		}
		slog.Info("OIDC authentication enabled", "provider_type", cfg.OIDCProviderType, "provider_name", oidcProvider.ProviderName())
	}

	// OAuth AS facade for MCP auth (lets Claude Code authenticate via browser).
	// Registered before auth middleware because OAuth endpoints are unauthenticated.
	if cfg.OIDCEnabled && oidcHandler != nil {
		oauthHandler := api.NewOAuthHandler(oidcHandler, oidcHandler.BaseURL())
		if authRateLimiter != nil {
			oauthHandler.RegisterRoutes(mux, authRateLimiter.Middleware(cfg.TrustXForwardedFor))
		} else {
			oauthHandler.RegisterRoutes(mux)
		}
		slog.Info("OAuth MCP auth enabled", "base_url", oidcHandler.BaseURL())
	}

	// Periodic cleanup: expired tokens (always) and device codes (when OIDC enabled)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := app.DBClient().DeleteExpiredTokens(serverCtx); err != nil {
					slog.Warn("failed to clean up expired tokens", "error", err)
					app.Metrics().RecordCleanupFailure("expired_tokens")
				}
				if cfg.OIDCEnabled {
					if err := app.DBClient().DeleteExpiredDeviceCodes(serverCtx); err != nil {
						slog.Warn("failed to clean up expired device codes", "error", err)
						app.Metrics().RecordCleanupFailure("expired_device_codes")
					}
					if err := app.DBClient().DeleteExpiredOAuthAuthCodes(serverCtx); err != nil {
						slog.Warn("failed to clean up expired oauth auth codes", "error", err)
						app.Metrics().RecordCleanupFailure("expired_oauth_auth_codes")
					}
				}
			case <-serverCtx.Done():
				return
			}
		}
	}()

	// Auth middleware
	var authMw func(http.Handler) http.Handler
	if cfg.NoAuth {
		slog.Warn("no-auth mode enabled: skipping token validation")
		authMw = auth.NoAuthMiddleware
	} else {
		authMw = auth.Middleware(app.DBClient(), app.Metrics(), auth.MiddlewareConfig{
			TrustXForwardedFor: cfg.TrustXForwardedFor,
		})
	}

	// REST API (includes agent routes)
	apiServer := api.NewServer(app)
	apiServer.Register(mux, authMw, app.AgentRunner())

	// MCP endpoint for Model Context Protocol (on main port, behind auth)
	if cfg.MCPEnabled {
		mcpHandler := mcptools.NewHandler(
			&tools.Executor{
				DB:        app.DBClient(),
				Search:    app.SearchService(),
				FileSvc:   app.FileService(),
				RenderSvc: app.RenderService(),
				Jina:      app.JinaClient(),
			},
			app.DBClient(),
			app.FileService(),
			app.VaultService(),
			app.RemoteService(),
			app.MemoryService(),
			app.ApifyClient(),
			app.JinaClient(),
		)
		mux.Handle("/mcp", authMw(mcpHandler))
		slog.Info("MCP endpoint enabled", "path", "/mcp")
	}

	// API documentation (Scalar UI at /, spec at /api/v1/openapi.yaml)
	if cfg.DocsEnabled {
		api.RegisterDocs(mux)
	} else {
		slog.Info("API docs disabled (KNOW_DOCS_ENABLED=false)")
	}

	// Document change events (SSE) — vault-scoped
	// Uses the apiServer's vault scope middleware for consistent vault resolution.
	mux.Handle("GET /api/v1/vaults/{vault}/events", authMw(apiServer.VaultScopeHandler(event.HandleEvents(app.EventBus()))))

	// WebDAV server — separate port (editors connect directly)
	var protoSrv *http.Server
	{
		protoMux := http.NewServeMux()

		var davHandler http.Handler = knowdav.NewHandler(
			serverCtx,
			"/dav/",
			app.DBClient(),
			app.FileService(),
			app.VaultService(),
			app.BlobStore(),
			cfg.NoAuth,
			10*1024*1024, // 10 MB max PUT body
			app.Metrics(),
		)

		// Optional debug log: KNOW_DAV_DEBUG_LOG=/tmp/dav-debug.log
		davDebugLog := os.Getenv("KNOW_DAV_DEBUG_LOG")
		if davDebugLog != "" {
			davLogger, davLogFile, logErr := knowdav.NewDebugLogger(davDebugLog)
			if logErr != nil {
				slog.Error("failed to open DAV debug log", "path", davDebugLog, "error", logErr)
			} else {
				defer davLogFile.Close()
				davHandler = knowdav.DebugLogMiddleware(davLogger, davHandler)
				slog.Info("WebDAV debug logging enabled", "path", davDebugLog)
			}
		}

		protoMux.Handle("/dav/", davHandler)
		slog.Info("WebDAV endpoint enabled", "port", cfg.ProtocolPort, "path", "/dav/{vaultName}/")

		protoLn, listenErr := net.Listen("tcp", listenHost+":"+cfg.ProtocolPort)
		if listenErr != nil {
			return fmt.Errorf("bind protocol port %s: %w", cfg.ProtocolPort, listenErr)
		}
		protoSrv = &http.Server{
			Handler:           protoMux,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       120 * time.Second,
		}
		go func() {
			if err := protoSrv.Serve(protoLn); err != nil && err != http.ErrServerClosed {
				slog.Error("protocol server error", "error", err)
			}
		}()
		slog.Info("WebDAV server started", "port", cfg.ProtocolPort)
	}

	// SSH/SFTP server (optional)
	var sshSrv *sshd.Server
	if cfg.SSHEnabled {
		sshLn, listenErr := net.Listen("tcp", listenHost+":"+cfg.SSHPort)
		if listenErr != nil {
			return fmt.Errorf("bind SSH port %s: %w", cfg.SSHPort, listenErr)
		}
		sshSrv, err = sshd.NewServer(
			sshLn,
			app.DBClient(),
			app.FileService(),
			app.VaultService(),
			cfg.SSHHostKeyPath,
			cfg.NoAuth,
		)
		if err != nil {
			return fmt.Errorf("create SSH server: %w", err)
		}
		go sshSrv.Serve()
		slog.Info("SSH/SFTP server enabled", "port", cfg.SSHPort)
	}

	// NFS server (optional, always binds to localhost)
	var nfsSrv *knownfs.Server
	if cfg.NFSEnabled {
		nfsLn, listenErr := net.Listen("tcp", "127.0.0.1:"+cfg.NFSPort)
		if listenErr != nil {
			return fmt.Errorf("bind NFS port %s: %w", cfg.NFSPort, listenErr)
		}
		nfsSrv = knownfs.NewServer(
			nfsLn,
			app.DBClient(),
			app.FileService(),
			app.VaultService(),
		)
		go nfsSrv.Serve()
		slog.Info("NFS server enabled (localhost only)", "port", cfg.NFSPort)
	}

	// Prometheus metrics server (optional)
	var metricsSrv *http.Server
	if cfg.MetricsPort != "" {
		metricsLn, listenErr := net.Listen("tcp", ":"+cfg.MetricsPort)
		if listenErr != nil {
			return fmt.Errorf("bind metrics port %s: %w", cfg.MetricsPort, listenErr)
		}
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsSrv = &http.Server{
			Handler:           metricsMux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			if err := metricsSrv.Serve(metricsLn); err != nil && err != http.ErrServerClosed {
				slog.Error("metrics server error", "error", err)
			}
		}()
		slog.Info("Prometheus metrics enabled", "port", cfg.MetricsPort, "url", fmt.Sprintf("http://0.0.0.0:%s/metrics", cfg.MetricsPort))
	}

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Global rate limiter (outermost middleware)
	var handler http.Handler = api.SecurityHeadersMiddleware(api.RequestLogMiddleware(app.Metrics(), mux))
	if cfg.RateLimitGlobalRPS > 0 {
		globalRL := api.NewIPRateLimiter(cfg.RateLimitGlobalRPS, cfg.RateLimitGlobalBurst, "global", app.Metrics())
		defer globalRL.Stop()
		handler = globalRL.Middleware(cfg.TrustXForwardedFor)(handler)
		slog.Info("global rate limiting enabled", "rps", cfg.RateLimitGlobalRPS, "burst", cfg.RateLimitGlobalBurst)
	}

	httpServer := &http.Server{
		Addr:              listenHost + ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout intentionally omitted: SSE endpoints are long-lived.
		// Each handler manages its own lifecycle via context cancellation and
		// write error checks.
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("REST API available", "url", fmt.Sprintf("http://localhost:%s/api/v1/", port))
		slog.Info("API docs available", "url", fmt.Sprintf("http://localhost:%s/", port))

		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			quit <- syscall.SIGTERM
		}
	}()
	<-quit

	slog.Info("shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	// 1. Stop SIGHUP listener to prevent reloads during shutdown
	slog.Info("stopping SIGHUP listener")
	signal.Stop(sighup)
	close(sighup)

	// 2. Cancel serverCtx to stop WebDAV background goroutines
	slog.Info("cancelling background goroutines")
	serverCancel()

	// 3. WebDAV server shutdown
	if protoSrv != nil {
		slog.Info("webdav: shutting down")
		if err := protoSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("webdav: shutdown error", "error", err)
		}
	}

	// 4. Metrics server shutdown
	if metricsSrv != nil {
		slog.Info("metrics: shutting down")
		if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("metrics: shutdown error", "error", err)
		}
	}

	// 5. NFS shutdown
	if nfsSrv != nil {
		slog.Info("nfs: shutting down")
		nfsSrv.Shutdown(shutdownCtx)
	}

	// 6. SSH shutdown — stop accepting new connections and drain active sessions
	if sshSrv != nil {
		slog.Info("ssh: shutting down")
		sshSrv.Shutdown(shutdownCtx)
		slog.Info("ssh: stopped")
	}

	// 7. HTTP shutdown — stop accepting new connections, drain in-flight requests
	slog.Info("http: shutting down")
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("http: shutdown error", "error", err)
	} else {
		slog.Info("http: stopped")
	}

	// 8. App shutdown — stop workers, agents, event bus, close DB (with deadline)
	slog.Info("stopping application services")
	if err := app.Close(shutdownCtx); err != nil {
		slog.Error("app close error", "error", err)
	}

	// Shutdown errors are logged but not returned: the server IS shutting down,
	// so a non-zero exit code would be misleading to process managers.
	slog.Info("server stopped")
	return nil
}
