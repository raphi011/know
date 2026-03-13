package db

import (
	"context"
	"fmt"
	"time"

	"github.com/surrealdb/surrealdb.go"
)

type checkpointRow struct {
	Data []byte `json:"data"`
}

func (c *Client) GetCheckpoint(ctx context.Context, id string) ([]byte, error) {
	defer c.logOp(ctx, "agent_checkpoint.get", time.Now())
	sql := `SELECT data FROM type::record("agent_checkpoint", $id)`
	results, err := surrealdb.Query[[]checkpointRow](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get checkpoint: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return (*results)[0].Result[0].Data, nil
}

func (c *Client) UpsertCheckpoint(ctx context.Context, id string, data []byte) error {
	defer c.logOp(ctx, "agent_checkpoint.upsert", time.Now())
	sql := `UPSERT type::record("agent_checkpoint", $id) SET data = $data, updated_at = time::now()`
	_, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id":   id,
		"data": data,
	})
	if err != nil {
		return fmt.Errorf("upsert checkpoint: %w", err)
	}
	return nil
}

func (c *Client) DeleteCheckpoint(ctx context.Context, id string) error {
	defer c.logOp(ctx, "agent_checkpoint.delete", time.Now())
	sql := `DELETE type::record("agent_checkpoint", $id)`
	_, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return fmt.Errorf("delete checkpoint: %w", err)
	}
	return nil
}
