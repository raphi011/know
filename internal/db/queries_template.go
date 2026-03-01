package db

import (
	"context"
	"fmt"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func (c *Client) CreateTemplate(ctx context.Context, input models.TemplateInput) (*models.Template, error) {
	var vaultVal any
	if input.VaultID != nil {
		vaultVal = newRecordID("vault", *input.VaultID)
	} else {
		vaultVal = surrealmodels.None
	}

	sql := `
		CREATE template SET
			vault = $vault,
			name = $name,
			description = $description,
			content = $content,
			is_ai_template = $is_ai_template
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Template](ctx, c.DB(), sql, map[string]any{
		"vault":          vaultVal,
		"name":           input.Name,
		"description":    optionalString(input.Description),
		"content":        input.Content,
		"is_ai_template": input.IsAITemplate,
	})
	if err != nil {
		return nil, fmt.Errorf("create template: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create template: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetTemplate(ctx context.Context, id string) (*models.Template, error) {
	sql := `SELECT * FROM type::record("template", $id)`
	results, err := surrealdb.Query[[]models.Template](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) ListTemplates(ctx context.Context, vaultID *string) ([]models.Template, error) {
	var sql string
	vars := map[string]any{}

	if vaultID != nil {
		sql = `SELECT * FROM template WHERE vault = type::record("vault", $vault_id) OR vault IS NONE ORDER BY name ASC`
		vars["vault_id"] = *vaultID
	} else {
		sql = `SELECT * FROM template ORDER BY name ASC`
	}

	results, err := surrealdb.Query[[]models.Template](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) DeleteTemplate(ctx context.Context, id string) error {
	sql := `DELETE type::record("template", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": id}); err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	return nil
}
