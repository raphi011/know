// Package main provides the GraphQL server for Knowhow.
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

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/config"
	"github.com/raphi011/knowhow/internal/event"
	"github.com/raphi011/knowhow/internal/graph"
	"github.com/raphi011/knowhow/internal/mcptools"
	"github.com/raphi011/knowhow/internal/sshd"
	"github.com/raphi011/knowhow/internal/tools"
	"github.com/raphi011/knowhow/internal/web"
	knowhowdav "github.com/raphi011/knowhow/internal/webdav"
	"github.com/vektah/gqlparser/v2/ast"
)

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

	slog.Info("starting knowhow-server", "port", port)

	// Bind the port early so we fail fast if another instance is still running.
	// This prevents the race where initialization succeeds but ListenAndServe
	// fails because the old process hasn't released the port yet.
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		slog.Error("failed to bind port", "port", port, "error", err)
		os.Exit(1)
	}

	// Create resolver with all dependencies
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	resolver, err := graph.NewResolver(ctx, cfg)
	cancel()
	if err != nil {
		slog.Error("failed to create resolver", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := resolver.Close(context.Background()); err != nil {
			slog.Error("failed to close resolver", "error", err)
		}
	}()

	// Create GraphQL server
	srv := handler.New(graph.NewExecutableSchema(graph.Config{
		Resolvers: resolver,
	}))

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	// Setup routes
	mux := http.NewServeMux()

	// Auth middleware wraps GraphQL handler
	var authMw func(http.Handler) http.Handler
	if cfg.NoAuth {
		slog.Warn("no-auth mode enabled: skipping token validation")
		authMw = auth.NoAuthMiddleware
	} else {
		authMw = auth.Middleware(resolver.DBClient())
	}
	mux.Handle("/query", authMw(srv))

	// Agent endpoints
	mux.Handle("/agent/chat", authMw(resolver.AgentService().HandleChat()))
	mux.Handle("/agent/approval", authMw(resolver.AgentService().HandleApproval()))

	// SSE endpoint for streaming document change events
	mux.Handle("/events", authMw(event.HandleEvents(resolver.EventBus())))

	// MCP endpoint for Model Context Protocol
	if cfg.MCPEnabled {
		mcpHandler := mcptools.NewHandler(
			&tools.Executor{
				DB:         resolver.DBClient(),
				Search:     resolver.SearchService(),
				DocService: resolver.DocumentService(),
			},
			resolver.DBClient(),
			resolver.VaultService(),
		)
		mux.Handle("/mcp", authMw(mcpHandler))
		slog.Info("MCP endpoint enabled", "path", "/mcp")
	}

	// WebDAV endpoint for document editing with any editor
	davHandler := knowhowdav.NewHandler(
		"/dav/",
		resolver.DBClient(),
		resolver.DocumentService(),
		resolver.VaultService(),
		cfg.NoAuth,
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
			resolver.DBClient(),
			resolver.DocumentService(),
			resolver.VaultService(),
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

	// Web UI (Templ + HTMX)
	webHandler := web.NewHandler(
		resolver.DBClient(),
		resolver.DocumentService(),
		resolver.VaultService(),
		resolver.SearchService(),
		resolver.EventBus(),
	)
	webHandler.Register(mux)
	slog.Info("Web UI enabled", "login", fmt.Sprintf("http://localhost:%s/login", port))

	// Playground is unauthenticated (for dev)
	mux.Handle("/playground", playground.Handler("Knowhow v2", "/query"))

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
		slog.Info("GraphQL playground available", "url", fmt.Sprintf("http://localhost:%s/playground", port))
		slog.Info("GraphQL endpoint available", "url", fmt.Sprintf("http://localhost:%s/query", port))

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
