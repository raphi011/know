package db

import (
	"context"
	"fmt"
	"log/slog"

	v1db "github.com/raphaelgruber/memcp-go/internal/db"
	"github.com/raphaelgruber/memcp-go/internal/metrics"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// Client wraps the v1 DB client, reusing its connection logic with the v2 schema.
type Client struct {
	*v1db.Client
}

// NewClient creates a new v2 DB client.
func NewClient(ctx context.Context, cfg v1db.Config, log *slog.Logger, mc *metrics.Collector) (*Client, error) {
	inner, err := v1db.NewClient(ctx, cfg, log, mc)
	if err != nil {
		return nil, err
	}
	return &Client{Client: inner}, nil
}

// InitSchema initializes the v2 database schema.
func (c *Client) InitSchema(ctx context.Context, embedDimension int) error {
	slog.Info("initializing v2 database schema", "embed_dimension", embedDimension)
	_, err := surrealdb.Query[any](ctx, c.DB(), SchemaSQL(embedDimension), nil)
	if err != nil {
		return fmt.Errorf("init v2 schema: %w", err)
	}
	return nil
}

// optionalString returns models.None for nil pointers, otherwise returns the string value.
func optionalString(s *string) any {
	if s == nil {
		return surrealmodels.None
	}
	return *s
}

// optionalObject returns models.None for nil maps, otherwise returns the map.
func optionalObject(m map[string]any) any {
	if m == nil {
		return surrealmodels.None
	}
	return m
}

// optionalEmbedding returns models.None for nil/empty slices, otherwise returns the slice.
func optionalEmbedding(e []float32) any {
	if len(e) == 0 {
		return surrealmodels.None
	}
	return e
}

// optionalRecordID returns models.None for nil record pointers, otherwise returns the record.
func optionalRecordID(r *surrealmodels.RecordID) any {
	if r == nil {
		return surrealmodels.None
	}
	return *r
}

// WipeData deletes all v2 data while preserving schema.
func (c *Client) WipeData(ctx context.Context) error {
	tables := []string{"doc_relation", "wiki_link", "chunk", "document", "template", "api_token", "vault", "user"}
	for _, table := range tables {
		query := fmt.Sprintf("DELETE %s", table)
		if _, err := surrealdb.Query[any](ctx, c.DB(), query, nil); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	return nil
}
