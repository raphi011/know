// Package main provides the GraphQL server for Knowhow.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
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
	"github.com/gorilla/websocket"
	"github.com/raphaelgruber/memcp-go/internal/config"
	"github.com/raphaelgruber/memcp-go/internal/graph"
	"github.com/raphaelgruber/memcp-go/web"
	"github.com/vektah/gqlparser/v2/ast"
)

func main() {
	// Parse flags
	wipeDB := flag.Bool("wipe", false, "wipe all data from database on startup (testing only)")
	flag.Parse()

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

	// Create resolver with all dependencies
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	resolver, err := graph.NewResolver(ctx, cfg)
	cancel()
	if err != nil {
		slog.Error("failed to create resolver", "error", err)
		os.Exit(1)
	}

	// Wipe database if requested (via flag or env var)
	if *wipeDB || os.Getenv("KNOWHOW_WIPE_DB") == "true" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := resolver.WipeData(ctx); err != nil {
			cancel()
			slog.Error("failed to wipe database", "error", err)
			os.Exit(1)
		}
		cancel()
	}
	defer func() {
		if err := resolver.Close(context.Background()); err != nil {
			slog.Error("failed to close resolver", "error", err)
		}
	}()

	// Create GraphQL server with explicit transports for WebSocket subscription support
	srv := handler.New(graph.NewExecutableSchema(graph.Config{
		Resolvers: resolver,
	}))

	// Add transports - order matters: WebSocket first for subscription upgrades
	srv.AddTransport(transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for local dev
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		KeepAlivePingInterval: 10 * time.Second,
	})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	// Add standard extensions
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	// Setup routes
	mux := http.NewServeMux()

	// GraphQL playground moved to /playground
	mux.Handle("/playground", playground.Handler("Knowhow GraphQL", "/query"))

	// GraphQL endpoint (no CORS needed: Vite proxy handles dev, same-origin handles prod)
	mux.Handle("/query", srv)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	// Serve embedded SPA from web/dist
	distFS, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		slog.Error("failed to create sub filesystem", "error", err)
		os.Exit(1)
	}
	fileServer := http.FileServer(http.FS(distFS))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Try serving the file directly; fall back to index.html for SPA routing
		if r.URL.Path != "/" {
			f, err := distFS.Open(r.URL.Path[1:])
			if errors.Is(err, fs.ErrNotExist) {
				r.URL.Path = "/"
			} else if err != nil {
				slog.Warn("unexpected error opening embedded file", "path", r.URL.Path, "error", err)
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			} else {
				f.Close()
			}
		}
		fileServer.ServeHTTP(w, r)
	})

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 60 * time.Second, // Long for LLM responses
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		slog.Info("Web UI available", "url", fmt.Sprintf("http://localhost:%s/", port))
		slog.Info("GraphQL playground available", "url", fmt.Sprintf("http://localhost:%s/playground", port))
		slog.Info("GraphQL endpoint available", "url", fmt.Sprintf("http://localhost:%s/query", port))

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}
