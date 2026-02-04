// Package db provides SurrealDB database connectivity with auto-reconnect support.
package db

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/surrealdb/surrealdb.go"
	"github.com/surrealdb/surrealdb.go/contrib/rews"
	"github.com/surrealdb/surrealdb.go/pkg/connection"
	"github.com/surrealdb/surrealdb.go/pkg/connection/gorillaws"
	"github.com/surrealdb/surrealdb.go/pkg/logger"
	"github.com/surrealdb/surrealdb.go/surrealcbor"
)

func init() {
	// Force HTTP/1.1 for WSS connections to prevent HTTP/2 ALPN negotiation.
	// WebSocket upgrade requires HTTP/1.1 semantics which fail under HTTP/2.
	gorillaws.DefaultDialer.TLSClientConfig = &tls.Config{
		NextProtos: []string{"http/1.1"},
	}
}

// Config holds SurrealDB connection configuration.
type Config struct {
	URL       string
	Namespace string
	Database  string
	Username  string
	Password  string
	AuthLevel string // "root" or "database"
}

// Client wraps SurrealDB connection with auto-reconnect.
type Client struct {
	conn   *rews.Connection[*gorillaws.Connection]
	db     *surrealdb.DB
	cfg    Config
	logger logger.Logger
}

// NewClient creates a new SurrealDB client with auto-reconnecting WebSocket.
func NewClient(ctx context.Context, cfg Config, log *slog.Logger) (*Client, error) {
	// Create logger adapter for SurrealDB SDK
	var sdkLogger logger.Logger
	if log != nil {
		sdkLogger = logger.New(log.Handler())
	} else {
		sdkLogger = logger.New(slog.Default().Handler())
	}

	// Use surrealcbor for CBOR encoding/decoding (handles SurrealDB custom tags)
	codec := surrealcbor.New()

	// Create rews connection with auto-reconnect using gorillaws
	// Note: gorillaws requires ws:// or wss:// URL without /rpc suffix (it adds /rpc internally)
	baseURL := cfg.URL
	if strings.HasSuffix(baseURL, "/rpc") {
		baseURL = strings.TrimSuffix(baseURL, "/rpc")
	}

	conn := rews.New(
		func(ctx context.Context) (*gorillaws.Connection, error) {
			ws := gorillaws.New(&connection.Config{
				BaseURL:     baseURL,
				Marshaler:   codec,
				Unmarshaler: codec,
				Logger:      sdkLogger,
			})
			return ws, nil
		},
		5*time.Second,
		codec,
		sdkLogger,
	)

	// Configure exponential backoff
	retryer := rews.NewExponentialBackoffRetryer()
	retryer.InitialDelay = 1 * time.Second
	retryer.MaxDelay = 30 * time.Second
	retryer.Multiplier = 2.0
	retryer.MaxRetries = 10
	conn.Retryer = retryer

	// Connect
	sdkLogger.Info("connecting to SurrealDB", "url", cfg.URL)
	if err := conn.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	// Create DB wrapper
	db, err := surrealdb.FromConnection(ctx, conn)
	if err != nil {
		_ = conn.Close(ctx)
		return nil, fmt.Errorf("from connection: %w", err)
	}

	// Authenticate based on auth level
	sdkLogger.Info("authenticating", "user", cfg.Username, "auth_level", cfg.AuthLevel)
	if cfg.AuthLevel == "database" {
		_, err = db.SignIn(ctx, surrealdb.Auth{
			Namespace: cfg.Namespace,
			Database:  cfg.Database,
			Username:  cfg.Username,
			Password:  cfg.Password,
		})
	} else {
		// Default to root auth
		_, err = db.SignIn(ctx, surrealdb.Auth{
			Username: cfg.Username,
			Password: cfg.Password,
		})
	}
	if err != nil {
		_ = conn.Close(ctx)
		return nil, fmt.Errorf("signin: %w", err)
	}

	// Select namespace/database
	sdkLogger.Info("selecting namespace/database", "namespace", cfg.Namespace, "database", cfg.Database)
	if err := db.Use(ctx, cfg.Namespace, cfg.Database); err != nil {
		_ = conn.Close(ctx)
		return nil, fmt.Errorf("use: %w", err)
	}

	sdkLogger.Info("SurrealDB connection established")
	return &Client{conn: conn, db: db, cfg: cfg, logger: sdkLogger}, nil
}

// Close closes the SurrealDB connection.
func (c *Client) Close(ctx context.Context) error {
	c.logger.Info("closing SurrealDB connection")
	return c.conn.Close(ctx)
}

// DB returns the underlying SurrealDB client for queries.
func (c *Client) DB() *surrealdb.DB {
	return c.db
}

// InitSchema initializes the database schema.
func (c *Client) InitSchema(ctx context.Context) error {
	c.logger.Info("initializing database schema")
	_, err := surrealdb.Query[any](ctx, c.db, SchemaSQL, nil)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	c.logger.Info("schema initialization complete")
	return nil
}

// Query executes a SurrealQL query with parameters.
// Returns the raw query results as []surrealdb.QueryResult[any].
func (c *Client) Query(ctx context.Context, sql string, vars map[string]any) (*[]surrealdb.QueryResult[any], error) {
	return surrealdb.Query[any](ctx, c.db, sql, vars)
}

// WipeData deletes all data from the database while preserving schema.
// Use for testing only.
func (c *Client) WipeData(ctx context.Context) error {
	c.logger.Warn("wiping all data from database")

	// Delete all records from each table
	// Order matters due to relations referencing entities
	tables := []string{"relates_to", "chunk", "template", "token_usage", "ingest_job", "entity"}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE %s", table)
		if _, err := surrealdb.Query[any](ctx, c.db, query, nil); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
		c.logger.Info("deleted table data", "table", table)
	}

	c.logger.Info("database wipe complete")
	return nil
}
