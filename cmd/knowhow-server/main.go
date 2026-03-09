// Package main provides the Knowhow server.
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
)

// Version information — set by GoReleaser ldflags at build time.
var version = "dev"

func main() {
	// Load configuration
	cfg := config.Load()

	// Get server port from environment or default
	port := os.Getenv("KNOWHOW_SERVER_PORT")
	if port == "" {
		port = "8484"
	}

	// Initialize logging
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	slog.Info("starting knowhow-server", "version", version, "port", port)

	// Bind the port early so we fail fast if another instance is still running.
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		slog.Error("failed to bind port", "port", port, "error", err)
		os.Exit(1)
	}

	// Create application with all dependencies
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	app, err := server.New(ctx, cfg)
	cancel()
	if err != nil {
		slog.Error("failed to create app", "error", err)
		os.Exit(1)
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
	davHandler := knowhowdav.NewHandler(
		"/dav/",
		app.DBClient(),
		app.DocumentService(),
		app.VaultService(),
		cfg.NoAuth,
		10*1024*1024, // 10 MB max PUT body
	)
	mux.Handle("/dav/", davHandler)
	slog.Info("WebDAV endpoint enabled", "path", "/dav/{vaultName}/")

	// SSH/SFTP server (optional)
	var sshSrv *sshd.Server
	if cfg.SSHEnabled {
		sshLn, err := net.Listen("tcp", ":"+cfg.SSHPort)
		if err != nil {
			slog.Error("failed to bind SSH port", "port", cfg.SSHPort, "error", err)
			os.Exit(1)
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
			slog.Error("failed to create SSH server", "error", err)
			os.Exit(1)
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
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
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
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
