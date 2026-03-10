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

	"github.com/raphi011/knowhow/internal/api"
	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/config"
	"github.com/raphi011/knowhow/internal/mcptools"
	"github.com/raphi011/knowhow/internal/server"
	"github.com/raphi011/knowhow/internal/sshd"
	"github.com/raphi011/knowhow/internal/tools"
	knowhowdav "github.com/raphi011/knowhow/internal/webdav"
	"github.com/spf13/cobra"
)

var (
	servePort     int
	serveNoAuth   bool
	serveSSH      bool
	serveSSHPort  int
	serveLogLevel string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Knowhow server",
	Long: `Start the Knowhow HTTP server with REST API, WebDAV, and optional MCP/SSH endpoints.

Configuration is loaded from environment variables (see config package).
Flags override the corresponding env vars.

Examples:
  knowhow serve
  knowhow serve --port 8080
  knowhow serve --no-auth --log-level debug
  knowhow serve --ssh --ssh-port 2222`,
	RunE: runServe,
}

func init() {
	serveCmd.Flags().IntVar(&servePort, "port", envOrDefaultInt("KNOWHOW_SERVER_PORT", 8484), "HTTP server port")
	serveCmd.Flags().BoolVar(&serveNoAuth, "no-auth", envOrDefaultBool("KNOWHOW_NO_AUTH", false), "disable token authentication")
	serveCmd.Flags().BoolVar(&serveSSH, "ssh", envOrDefaultBool("KNOWHOW_SSH_ENABLED", false), "enable SSH/SFTP server")
	serveCmd.Flags().IntVar(&serveSSHPort, "ssh-port", envOrDefaultInt("KNOWHOW_SSH_PORT", 2222), "SSH server port")
	serveCmd.Flags().StringVar(&serveLogLevel, "log-level", envOrDefault("LOG_LEVEL", "info"), "log level (debug, info, warn, error)")
}

func runServe(_ *cobra.Command, _ []string) error {
	// Initialize logging
	level := slog.LevelInfo
	if serveLogLevel == "debug" {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	port := fmt.Sprintf("%d", servePort)
	slog.Info("starting knowhow server", "version", version, "port", port)

	// Bind the port early so we fail fast if another instance is still running.
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("bind port %s: %w", port, err)
	}

	// Load configuration and apply flag overrides
	cfg := config.Load()
	cfg.NoAuth = serveNoAuth
	cfg.SSHEnabled = serveSSH
	cfg.SSHPort = fmt.Sprintf("%d", serveSSHPort)

	// Create application with all dependencies
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	app, err := server.New(ctx, cfg)
	cancel()
	if err != nil {
		return fmt.Errorf("create app: %w", err)
	}
	defer func() {
		if err := app.Close(context.Background()); err != nil {
			slog.Error("failed to close app", "error", err)
		}
	}()

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
				slog.Info("SIGHUP received, reloading LLM config")
				if err := app.ReloadLLM(); err != nil {
					slog.Warn("LLM reload failed", "error", err)
				}
			}()
		}
	}()

	// Setup routes
	mux := http.NewServeMux()

	// Auth middleware
	var authMw func(http.Handler) http.Handler
	if cfg.NoAuth {
		slog.Warn("no-auth mode enabled: skipping token validation")
		authMw = auth.NoAuthMiddleware
	} else {
		authMw = auth.Middleware(app.DBClient())
	}

	// Agent endpoints
	mux.Handle("/agent/chat", authMw(app.AgentService().HandleChat()))
	mux.Handle("/agent/approval", authMw(app.AgentService().HandleApproval()))

	// REST API
	apiServer := api.NewServer(app)
	apiServer.Register(mux, authMw)

	// MCP endpoint for Model Context Protocol
	if cfg.MCPEnabled {
		mcpHandler := mcptools.NewHandler(
			&tools.Executor{
				DB:         app.DBClient(),
				Search:     app.SearchService(),
				DocService: app.DocumentService(),
			},
			app.DBClient(),
			app.VaultService(),
		)
		mux.Handle("/mcp", authMw(mcpHandler))
		slog.Info("MCP endpoint enabled", "path", "/mcp")
	}

	// WebDAV endpoint for document editing with any editor
	var davHandler http.Handler = knowhowdav.NewHandler(
		"/dav/",
		app.DBClient(),
		app.DocumentService(),
		app.VaultService(),
		cfg.NoAuth,
		10*1024*1024, // 10 MB max PUT body
	)

	// Optional debug log: KNOWHOW_DAV_DEBUG_LOG=/tmp/dav-debug.log
	davDebugLog := os.Getenv("KNOWHOW_DAV_DEBUG_LOG")
	if davDebugLog != "" {
		davLogger, davLogFile, logErr := knowhowdav.NewDebugLogger(davDebugLog)
		if logErr != nil {
			slog.Error("failed to open DAV debug log", "path", davDebugLog, "error", logErr)
		} else {
			defer davLogFile.Close()
			davHandler = knowhowdav.DebugLogMiddleware(davLogger, davHandler)
			slog.Info("WebDAV debug logging enabled", "path", davDebugLog)
		}
	}

	mux.Handle("/dav/", davHandler)
	slog.Info("WebDAV endpoint enabled", "path", "/dav/{vaultName}/")

	// SSH/SFTP server (optional)
	var sshSrv *sshd.Server
	if cfg.SSHEnabled {
		sshLn, listenErr := net.Listen("tcp", ":"+cfg.SSHPort)
		if listenErr != nil {
			return fmt.Errorf("bind SSH port %s: %w", cfg.SSHPort, listenErr)
		}
		sshSrv, err = sshd.NewServer(
			sshLn,
			app.DBClient(),
			app.DocumentService(),
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

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("REST API available", "url", fmt.Sprintf("http://localhost:%s/api/", port))

		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if sshSrv != nil {
		sshSrv.Shutdown(ctx)
	}

	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	slog.Info("server stopped")
	return nil
}
