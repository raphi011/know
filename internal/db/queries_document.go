package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

func (c *Client) CreateDocument(ctx context.Context, input models.DocumentInput) (*models.Document, error) {
	labels := input.Labels
	if labels == nil {
		labels = []string{}
	}

	sql := `
		CREATE document SET
			vault = type::record("vault", $vault_id),
			path = $path,
			title = $title,
			content = $content,
			content_body = $content_body,
			labels = $labels,
			doc_type = $doc_type,
			source = $source,
			source_path = $source_path,
			content_hash = $content_hash,
			metadata = $metadata
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"vault_id":     input.VaultID,
		"path":         input.Path,
		"title":        input.Title,
		"content":      input.Content,
		"content_body": input.ContentBody,
		"labels":       labels,
		"doc_type":     optionalString(input.DocType),
		"source":       string(input.Source),
		"source_path":  optionalString(input.SourcePath),
		"content_hash": optionalString(input.ContentHash),
		"metadata":     optionalObject(input.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("create document: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("create document: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetDocumentByPath(ctx context.Context, vaultID, path string) (*models.Document, error) {
	sql := `SELECT * FROM document WHERE vault = type::record("vault", $vault_id) AND path = $path LIMIT 1`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"vault_id": vaultID,
		"path":     path,
	})
	if err != nil {
		return nil, fmt.Errorf("get document by path: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) GetDocumentByID(ctx context.Context, id string) (*models.Document, error) {
	sql := `SELECT * FROM type::record("document", $id)`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get document by id: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

type ListDocumentsFilter struct {
	VaultID string
	Folder  *string
	Labels  []string
	DocType *string
	Limit   int
	Offset  int
}

func (c *Client) ListDocuments(ctx context.Context, filter ListDocumentsFilter) ([]models.Document, error) {
	var conditions []string
	vars := map[string]any{
		"vault_id": filter.VaultID,
	}

	conditions = append(conditions, `vault = type::record("vault", $vault_id)`)

	if filter.Folder != nil {
		conditions = append(conditions, `string::starts_with(path, $folder)`)
		vars["folder"] = *filter.Folder
	}
	if len(filter.Labels) > 0 {
		conditions = append(conditions, `labels CONTAINSANY $labels`)
		vars["labels"] = filter.Labels
	}
	if filter.DocType != nil {
		conditions = append(conditions, `doc_type = $doc_type`)
		vars["doc_type"] = *filter.DocType
	}

	limit := 100
	if filter.Limit > 0 {
		limit = filter.Limit
	}

	sql := fmt.Sprintf("SELECT * FROM document WHERE %s ORDER BY path ASC LIMIT %d START %d",
		strings.Join(conditions, " AND "), limit, filter.Offset)

	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) UpdateDocument(ctx context.Context, id string, content, contentBody, title string, labels []string, contentHash *string, metadata map[string]any) (*models.Document, error) {
	if labels == nil {
		labels = []string{}
	}

	sql := `
		UPDATE type::record("document", $id) SET
			content = $content,
			content_body = $content_body,
			title = $title,
			labels = $labels,
			content_hash = $content_hash,
			metadata = $metadata
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"id":           id,
		"content":      content,
		"content_body": contentBody,
		"title":        title,
		"labels":       labels,
		"content_hash": optionalString(contentHash),
		"metadata":     optionalObject(metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("update document: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("update document: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

func (c *Client) DeleteDocument(ctx context.Context, id string) error {
	sql := `DELETE type::record("document", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": id}); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	return nil
}

func (c *Client) MoveDocument(ctx context.Context, id, newPath string) (*models.Document, error) {
	sql := `
		UPDATE type::record("document", $id) SET
			path = $new_path
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"id":       id,
		"new_path": newPath,
	})
	if err != nil {
		return nil, fmt.Errorf("move document: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, fmt.Errorf("move document: no result returned")
	}
	return &(*results)[0].Result[0], nil
}

// UpsertDocument creates or updates a document by vault+path.
func (c *Client) UpsertDocument(ctx context.Context, input models.DocumentInput) (*models.Document, bool, error) {
	existing, err := c.GetDocumentByPath(ctx, input.VaultID, input.Path)
	if err != nil {
		return nil, false, fmt.Errorf("check existing document: %w", err)
	}

	if existing == nil {
		doc, err := c.CreateDocument(ctx, input)
		if err != nil {
			return nil, false, err
		}
		return doc, true, nil
	}

	idStr, err := models.RecordIDString(existing.ID)
	if err != nil {
		return nil, false, fmt.Errorf("extract document id: %w", err)
	}

	doc, err := c.UpdateDocument(ctx, idStr, input.Content, input.ContentBody, input.Title, input.Labels, input.ContentHash, input.Metadata)
	if err != nil {
		return nil, false, err
	}
	return doc, false, nil
}
