package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// SyncMeta is lightweight document metadata used for sync.
type SyncMeta struct {
	ID          surrealmodels.RecordID `json:"id"`
	Path        string                 `json:"path"`
	ContentHash *string                `json:"content_hash"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// SyncTombstone represents a deleted document for sync.
type SyncTombstone struct {
	DocID     string    `json:"doc_id"`
	Path      string    `json:"path"`
	DeletedAt time.Time `json:"deleted_at"`
}

// ListSyncMetadata returns lightweight metadata for documents in a vault,
// optionally filtered by updated_at > since. Used for incremental sync.
func (c *Client) ListSyncMetadata(ctx context.Context, vaultID string, since *string, limit, offset int) ([]SyncMeta, error) {
	if limit <= 0 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	vars := map[string]any{
		"vault_id": bareID("vault", vaultID),
		"limit":    limit,
		"offset":   offset,
	}

	var sql string
	if since != nil {
		sql = `SELECT id, path, content_hash ?? null AS content_hash, updated_at FROM document WHERE vault = type::record("vault", $vault_id) AND updated_at > type::datetime($since) ORDER BY path ASC LIMIT $limit START $offset`
		vars["since"] = *since
	} else {
		sql = `SELECT id, path, content_hash ?? null AS content_hash, updated_at FROM document WHERE vault = type::record("vault", $vault_id) ORDER BY path ASC LIMIT $limit START $offset`
	}

	results, err := surrealdb.Query[[]SyncMeta](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list sync metadata: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// ListTombstones returns tombstones for deleted documents in a vault since the given time.
func (c *Client) ListTombstones(ctx context.Context, vaultID string, since string, limit, offset int) ([]SyncTombstone, error) {
	if limit <= 0 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	sql := `SELECT doc_id, path, deleted_at FROM document_tombstone WHERE vault = type::record("vault", $vault_id) AND deleted_at > type::datetime($since) ORDER BY deleted_at ASC LIMIT $limit START $offset`
	results, err := surrealdb.Query[[]SyncTombstone](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"since":    since,
		"limit":    limit,
		"offset":   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list tombstones: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

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
			content_length = $content_length,
			labels = $labels,
			doc_type = $doc_type,
			source = $source,
			source_path = $source_path,
			content_hash = $content_hash,
			metadata = $metadata,
			processed = false
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"vault_id":       bareID("vault", input.VaultID),
		"path":           input.Path,
		"title":          input.Title,
		"content":        input.Content,
		"content_body":   input.ContentBody,
		"content_length": len(input.Content),
		"labels":         labels,
		"doc_type":       optionalString(input.DocType),
		"source":         string(input.Source),
		"source_path":    optionalString(input.SourcePath),
		"content_hash":   optionalString(input.ContentHash),
		"metadata":       optionalObject(input.Metadata),
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
		"vault_id": bareID("vault", vaultID),
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

// buildDocumentFilter constructs the WHERE clause, variables, and pagination
// suffix shared by ListDocuments and ListDocumentMetas.
func buildDocumentFilter(filter ListDocumentsFilter) (whereClause string, vars map[string]any, suffix string) {
	var conditions []string
	vars = map[string]any{
		"vault_id": bareID("vault", filter.VaultID),
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

	whereClause = strings.Join(conditions, " AND ")
	suffix = fmt.Sprintf("ORDER BY path ASC LIMIT %d START %d", limit, filter.Offset)
	return
}

func (c *Client) ListDocuments(ctx context.Context, filter ListDocumentsFilter) ([]models.Document, error) {
	where, vars, suffix := buildDocumentFilter(filter)
	sql := fmt.Sprintf("SELECT * FROM document WHERE %s %s", where, suffix)

	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

func (c *Client) UpdateDocument(ctx context.Context, id string, content, contentBody, title string, source models.DocumentSource, labels []string, contentHash *string, metadata map[string]any) (*models.Document, error) {
	if labels == nil {
		labels = []string{}
	}

	sql := `
		UPDATE type::record("document", $id) SET
			content = $content,
			content_body = $content_body,
			content_length = $content_length,
			title = $title,
			source = $source,
			labels = $labels,
			content_hash = $content_hash,
			metadata = $metadata,
			processed = false
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"id":             id,
		"content":        content,
		"content_body":   contentBody,
		"content_length": len(content),
		"title":          title,
		"source":         string(source),
		"labels":         labels,
		"content_hash":   optionalString(contentHash),
		"metadata":       optionalObject(metadata),
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

// DeleteDocumentsByPrefix deletes all documents in a vault whose path starts with the given prefix.
// Returns the number of deleted documents.
func (c *Client) DeleteDocumentsByPrefix(ctx context.Context, vaultID, pathPrefix string) (int, error) {
	sql := `DELETE FROM document WHERE vault = type::record("vault", $vault_id) AND string::starts_with(path, $prefix) RETURN BEFORE`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   pathPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("delete documents by prefix: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
}

// MoveDocumentsByPrefix updates all documents in a vault whose path starts with oldPrefix,
// replacing oldPrefix with newPrefix. Returns the count of moved documents.
func (c *Client) MoveDocumentsByPrefix(ctx context.Context, vaultID, oldPrefix, newPrefix string) (int, error) {
	sql := `
		UPDATE document
		SET path = string::concat($new_prefix, string::slice(path, string::len($old_prefix)))
		WHERE vault = type::record("vault", $vault_id)
		  AND string::starts_with(path, $old_prefix)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_prefix": oldPrefix,
		"new_prefix": newPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("move documents by prefix: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return 0, nil
	}
	return len((*results)[0].Result), nil
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

// ListUnprocessedDocuments returns documents that have not yet been processed by the async worker.
func (c *Client) ListUnprocessedDocuments(ctx context.Context, limit int) ([]models.Document, error) {
	if limit <= 0 {
		limit = 20
	}
	sql := `SELECT * FROM document WHERE processed = false ORDER BY updated_at ASC LIMIT $limit`
	results, err := surrealdb.Query[[]models.Document](ctx, c.DB(), sql, map[string]any{
		"limit": limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list unprocessed documents: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// MarkDocumentProcessed sets processed = true on a document.
func (c *Client) MarkDocumentProcessed(ctx context.Context, docID string) error {
	sql := `UPDATE type::record("document", $id) SET processed = true`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": docID}); err != nil {
		return fmt.Errorf("mark document processed: %w", err)
	}
	return nil
}

// ListLabels returns all distinct labels used by documents in the given vault.
func (c *Client) ListLabels(ctx context.Context, vaultID string) ([]string, error) {
	sql := `RETURN array::distinct(array::flatten(
		(SELECT labels FROM document WHERE vault = type::record("vault", $vault_id)).labels
	))`
	results, err := surrealdb.Query[[]string](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return []string{}, nil
	}
	labels := (*results)[0].Result
	if labels == nil {
		return []string{}, nil
	}
	return labels, nil
}

// GetDocumentMetaByPath returns lightweight metadata for a document (no content).
// Returns nil if the document doesn't exist.
func (c *Client) GetDocumentMetaByPath(ctx context.Context, vaultID, path string) (*models.DocumentMeta, error) {
	sql := `SELECT path, content_length, content_hash ?? null AS content_hash, updated_at FROM document WHERE vault = type::record("vault", $vault_id) AND path = $path LIMIT 1`
	results, err := surrealdb.Query[[]models.DocumentMeta](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     path,
	})
	if err != nil {
		return nil, fmt.Errorf("get document meta by path: %w", err)
	}
	if results == nil || len(*results) == 0 || len((*results)[0].Result) == 0 {
		return nil, nil
	}
	return &(*results)[0].Result[0], nil
}

// ListDocumentMetas returns lightweight metadata (no content) for documents matching the filter.
func (c *Client) ListDocumentMetas(ctx context.Context, filter ListDocumentsFilter) ([]models.DocumentMeta, error) {
	where, vars, suffix := buildDocumentFilter(filter)
	sql := fmt.Sprintf("SELECT path, content_length, content_hash ?? null AS content_hash, updated_at FROM document WHERE %s %s", where, suffix)

	results, err := surrealdb.Query[[]models.DocumentMeta](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list document metas: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return nil, nil
	}
	return (*results)[0].Result, nil
}

// UpsertDocument creates or updates a document by vault+path.
// On update, previousDoc contains the document state before the update (for versioning).
// Handles concurrent creates gracefully: if CreateDocument hits a unique constraint
// violation (another request created the same path between our check and insert),
// we retry as an update.
func (c *Client) UpsertDocument(ctx context.Context, input models.DocumentInput) (doc *models.Document, created bool, previousDoc *models.Document, err error) {
	const maxRetries = 3

	for attempt := range maxRetries {
		existing, err := c.GetDocumentByPath(ctx, input.VaultID, input.Path)
		if err != nil {
			return nil, false, nil, fmt.Errorf("check existing document: %w", err)
		}

		if existing == nil {
			doc, err := c.CreateDocument(ctx, input)
			if err != nil {
				// Unique constraint violation — another concurrent request created
				// the same vault+path between our check and insert.
				// Re-check so the next iteration finds the existing document.
				if isUniqueViolation(err) && attempt < maxRetries-1 {
					continue
				}
				return nil, false, nil, err
			}
			return doc, true, nil, nil
		}

		// Found existing document — update it
		idStr, err := models.RecordIDString(existing.ID)
		if err != nil {
			return nil, false, nil, fmt.Errorf("extract document id: %w", err)
		}

		doc, err = c.UpdateDocument(ctx, idStr, input.Content, input.ContentBody, input.Title, input.Source, input.Labels, input.ContentHash, input.Metadata)
		if err != nil {
			return nil, false, nil, err
		}
		return doc, false, existing, nil
	}

	// Should not be reached — the loop always returns or continues
	return nil, false, nil, fmt.Errorf("upsert exhausted %d retries due to concurrent writes", maxRetries)
}
