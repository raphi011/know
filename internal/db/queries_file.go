package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// idPath is used by stem-recomputation queries that only need the record ID and path.
type idPath struct {
	ID   surrealmodels.RecordID `json:"id"`
	Path string                 `json:"path"`
}

// fileSize returns the byte size for a file input. It checks (in order):
// an explicit Size override, len(Data) for binary files, and len(Content)
// for text files. The override is used by streaming imports where the file data
// is not buffered in memory.
func fileSize(input models.FileInput) int {
	if input.Size > 0 {
		return input.Size
	}
	if len(input.Data) > 0 {
		return len(input.Data)
	}
	return len(input.Content)
}

// ---------------------------------------------------------------------------
// Order / filter types
// ---------------------------------------------------------------------------

// FileOrderBy defines allowed ORDER BY clauses for file queries.
type FileOrderBy string

const (
	OrderByPathAsc       FileOrderBy = "path ASC"
	OrderByUpdatedAtDesc FileOrderBy = "updated_at DESC"
	OrderByUpdatedAtAsc  FileOrderBy = "updated_at ASC"
	OrderByCreatedAtDesc FileOrderBy = "created_at DESC"
)

var validOrderBy = map[FileOrderBy]bool{
	OrderByPathAsc:       true,
	OrderByUpdatedAtDesc: true,
	OrderByUpdatedAtAsc:  true,
	OrderByCreatedAtDesc: true,
}

// ListFilesFilter controls filtering, ordering, and pagination for file list queries.
type ListFilesFilter struct {
	VaultID      string
	Folder       *string
	Labels       []string
	DocType      *string
	MimeType     *string
	IsFolder     *bool
	UpdatedSince *time.Time  // only return files updated at or after this timestamp
	OrderBy      FileOrderBy // defaults to OrderByPathAsc
	Limit        int
	Offset       int
}

// buildFileFilter constructs the WHERE clause, variables, and pagination
// suffix shared by ListFiles and ListFileMetas.
func buildFileFilter(filter ListFilesFilter) (whereClause string, vars map[string]any, suffix string, err error) {
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
	if filter.MimeType != nil {
		conditions = append(conditions, `mime_type = $mime_type`)
		vars["mime_type"] = *filter.MimeType
	}
	if filter.IsFolder != nil {
		conditions = append(conditions, `is_folder = $is_folder`)
		vars["is_folder"] = *filter.IsFolder
	}
	if filter.UpdatedSince != nil {
		// Format as RFC3339Nano string — passing time.Time directly loses sub-second
		// precision in the SurrealDB Go SDK's CBOR encoding.
		conditions = append(conditions, `updated_at >= <datetime>$updated_since`)
		vars["updated_since"] = filter.UpdatedSince.Format(time.RFC3339Nano)
	}

	limit := 100
	if filter.Limit > 0 {
		limit = filter.Limit
	}

	orderBy := OrderByPathAsc
	if filter.OrderBy != "" {
		if !validOrderBy[filter.OrderBy] {
			return "", nil, "", fmt.Errorf("unsupported order by: %q", string(filter.OrderBy))
		}
		orderBy = filter.OrderBy
	}

	whereClause = strings.Join(conditions, " AND ")
	suffix = fmt.Sprintf("ORDER BY %s LIMIT %d START %d", string(orderBy), limit, filter.Offset)
	return
}

// ---------------------------------------------------------------------------
// File CRUD
// ---------------------------------------------------------------------------

func (c *Client) CreateFile(ctx context.Context, input models.FileInput) (*models.File, error) {
	defer c.logOp(ctx, "file.create", time.Now())
	labels := input.Labels
	if labels == nil {
		labels = []string{}
	}

	stem := ""
	if !input.IsFolder {
		stem = models.FilenameStem(input.Path)
	}

	sql := `
		CREATE file SET
			vault = type::record("vault", $vault_id),
			path = $path,
			title = $title,
			stem = $stem,
			size = $size,
			labels = $labels,
			doc_type = $doc_type,
			hash = $hash,
			metadata = $metadata,
			is_folder = $is_folder,
			mime_type = $mime_type
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id":  bareID("vault", input.VaultID),
		"path":      input.Path,
		"title":     input.Title,
		"stem":      stem,
		"size":      fileSize(input),
		"labels":    labels,
		"doc_type":  optionalString(input.DocType),
		"hash":      optionalString(input.Hash),
		"metadata":  optionalObject(input.Metadata),
		"is_folder": input.IsFolder,
		"mime_type": input.MimeType,
	})
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}
	return firstResult(results, "create file")
}

func (c *Client) GetFileByPath(ctx context.Context, vaultID, path string) (*models.File, error) {
	defer c.logOp(ctx, "file.get_by_path", time.Now())
	sql := `SELECT * FROM file WHERE vault = type::record("vault", $vault_id) AND path = $path LIMIT 1`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     path,
	})
	if err != nil {
		return nil, fmt.Errorf("get file by path: %w", err)
	}
	return firstResultOpt(results), nil
}

func (c *Client) GetFileByID(ctx context.Context, id string) (*models.File, error) {
	defer c.logOp(ctx, "file.get_by_id", time.Now())
	sql := `SELECT * FROM type::record("file", $id)`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get file by id: %w", err)
	}
	return firstResultOpt(results), nil
}

// ListFiles returns files matching the filter. By default only non-folder files are returned.
func (c *Client) ListFiles(ctx context.Context, filter ListFilesFilter) ([]models.File, error) {
	defer c.logOp(ctx, "file.list", time.Now())
	// Default to non-folder files only.
	if filter.IsFolder == nil {
		filter.IsFolder = new(false)
	}
	where, vars, suffix, err := buildFileFilter(filter)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	sql := fmt.Sprintf("SELECT * FROM file WHERE %s %s", where, suffix)

	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	return allResults(results), nil
}

func (c *Client) UpdateFile(ctx context.Context, id string, input models.FileInput) (*models.File, error) {
	defer c.logOp(ctx, "file.update", time.Now())
	labels := input.Labels
	if labels == nil {
		labels = []string{}
	}

	stem := ""
	if !input.IsFolder {
		stem = models.FilenameStem(input.Path)
	}

	sql := `
		UPDATE type::record("file", $id) SET
			size = $size,
			title = $title,
			stem = $stem,
			labels = $labels,
			hash = $hash,
			metadata = $metadata,
			mime_type = $mime_type
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"id":        id,
		"size":      fileSize(input),
		"title":     input.Title,
		"stem":      stem,
		"labels":    labels,
		"hash":      optionalString(input.Hash),
		"metadata":  optionalObject(input.Metadata),
		"mime_type": input.MimeType,
	})
	if err != nil {
		return nil, fmt.Errorf("update file: %w", err)
	}
	return firstResult(results, "update file")
}

// UpdateFileLabels sets the labels on a file identified by record ID.
func (c *Client) UpdateFileLabels(ctx context.Context, id string, labels []string) error {
	defer c.logOp(ctx, "file.update_labels", time.Now())
	if labels == nil {
		labels = []string{}
	}
	sql := `UPDATE type::record("file", $id) SET labels = $labels`
	_, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"id":     bareID("file", id),
		"labels": labels,
	})
	if err != nil {
		return fmt.Errorf("update file labels: %w", err)
	}
	return nil
}

// updateFileByPath updates a file identified by vault+path instead of record ID.
// Used as a fallback when the unique index reports a conflict but GetFileByPath
// cannot find the record (e.g. SurrealDB index/data inconsistency).
func (c *Client) updateFileByPath(ctx context.Context, input models.FileInput) (*models.File, error) {
	defer c.logOp(ctx, "file.update_by_path", time.Now())
	labels := input.Labels
	if labels == nil {
		labels = []string{}
	}

	stem := ""
	if !input.IsFolder {
		stem = models.FilenameStem(input.Path)
	}

	sql := `
		UPDATE file SET
			size = $size,
			title = $title,
			stem = $stem,
			labels = $labels,
			hash = $hash,
			metadata = $metadata,
			mime_type = $mime_type
		WHERE vault = type::record("vault", $vault_id) AND path = $path
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id":  bareID("vault", input.VaultID),
		"path":      input.Path,
		"size":      fileSize(input),
		"title":     input.Title,
		"stem":      stem,
		"labels":    labels,
		"hash":      optionalString(input.Hash),
		"metadata":  optionalObject(input.Metadata),
		"mime_type": input.MimeType,
	})
	if err != nil {
		return nil, fmt.Errorf("update file by path: %w", err)
	}
	return firstResult(results, "update file by path")
}

func (c *Client) DeleteFile(ctx context.Context, id string) error {
	defer c.logOp(ctx, "file.delete", time.Now())
	sql := `DELETE type::record("file", $id)`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": id}); err != nil {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

// DeleteFileAtomic removes a file and all its associated data (wiki links, label edges, chunks)
// in a single transaction. Cascade events remain as a safety net but the primary cleanup is
// synchronous and atomic. Post-commit steps (stem collision resolution, blob deletion, events)
// must be handled by the caller.
func (c *Client) DeleteFileAtomic(ctx context.Context, fileID string) error {
	defer c.logOp(ctx, "file.delete_atomic", time.Now())
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete file atomic begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit

	vars := map[string]any{"file_id": bareID("file", fileID)}
	for _, step := range []struct {
		name string
		sql  string
	}{
		{"delete wiki links", `DELETE FROM wiki_link WHERE from_file = type::record("file", $file_id)`},
		{"unresolve incoming wiki links", `UPDATE wiki_link SET to_file = NONE WHERE to_file = type::record("file", $file_id)`},
		{"delete label edges", `DELETE FROM has_label WHERE in = type::record("file", $file_id)`},
		{"delete chunks", `DELETE FROM chunk WHERE file = type::record("file", $file_id)`},
		{"delete file", `DELETE type::record("file", $file_id)`},
	} {
		if _, err := surrealdb.Query[any](ctx, tx, step.sql, vars); err != nil {
			return fmt.Errorf("delete file atomic %s: %w", step.name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("delete file atomic commit: %w", err)
	}
	return nil
}

// DeleteFilesByPrefix deletes all non-folder files in a vault whose path starts with the given prefix.
// Returns the number of deleted files. Folder cleanup is handled separately by DeleteFoldersByPrefix.
func (c *Client) DeleteFilesByPrefix(ctx context.Context, vaultID, pathPrefix string) (int, error) {
	defer c.logOp(ctx, "file.delete_by_prefix", time.Now())
	sql := `DELETE FROM file WHERE vault = type::record("vault", $vault_id) AND is_folder = false AND string::starts_with(path, $prefix) RETURN BEFORE`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   pathPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("delete files by prefix: %w", err)
	}
	return countResults(results), nil
}

// MoveFilesByPrefix updates all non-folder files in a vault whose path starts with oldPrefix,
// replacing oldPrefix with newPrefix. Returns the count of moved files.
// Folder moves are handled separately by MoveFoldersByPrefix.
func (c *Client) MoveFilesByPrefix(ctx context.Context, vaultID, oldPrefix, newPrefix string) (int, error) {
	defer c.logOp(ctx, "file.move_by_prefix", time.Now())
	sql := `
		UPDATE file
		SET path = string::concat($new_prefix, string::slice(path, string::len($old_prefix)))
		WHERE vault = type::record("vault", $vault_id)
		  AND is_folder = false
		  AND string::starts_with(path, $old_prefix)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_prefix": oldPrefix,
		"new_prefix": newPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("move files by prefix: %w", err)
	}
	return countResults(results), nil
}

// RecomputeStems updates the stem field for all non-folder files in a vault whose path
// starts with pathPrefix. Used after prefix-based moves to keep stems consistent.
func (c *Client) RecomputeStems(ctx context.Context, vaultID, pathPrefix string) error {
	defer c.logOp(ctx, "file.recompute_stems", time.Now())

	// Fetch affected files
	sql := `SELECT id, path FROM file WHERE vault = type::record("vault", $vault_id) AND is_folder = false AND string::starts_with(path, $prefix)`
	results, err := surrealdb.Query[[]idPath](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   pathPrefix,
	})
	if err != nil {
		return fmt.Errorf("recompute stems fetch: %w", err)
	}
	rows := allResults(results)
	if len(rows) == 0 {
		return nil
	}

	// Batch all stem updates into a single transaction (avoids N round-trips).
	var b strings.Builder
	vars := make(map[string]any, len(rows)*2)
	b.WriteString("BEGIN TRANSACTION;\n")
	for i, row := range rows {
		idStr, err := models.RecordIDString(row.ID)
		if err != nil {
			return fmt.Errorf("recompute stems extract id: %w", err)
		}
		idKey := fmt.Sprintf("id_%d", i)
		stemKey := fmt.Sprintf("stem_%d", i)
		fmt.Fprintf(&b, "UPDATE type::record(\"file\", $%s) SET stem = $%s;\n", idKey, stemKey)
		vars[idKey] = idStr
		vars[stemKey] = models.FilenameStem(row.Path)
	}
	b.WriteString("COMMIT TRANSACTION;")

	if _, err := surrealdb.Query[any](ctx, c.DB(), b.String(), vars); err != nil {
		return fmt.Errorf("recompute stems batch update: %w", err)
	}
	return nil
}

func (c *Client) MoveFile(ctx context.Context, id, newPath string) (*models.File, error) {
	defer c.logOp(ctx, "file.move", time.Now())
	sql := `
		UPDATE type::record("file", $id) SET
			path = $new_path,
			stem = $stem
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"id":       id,
		"new_path": newPath,
		"stem":     models.FilenameStem(newPath),
	})
	if err != nil {
		return nil, fmt.Errorf("move file: %w", err)
	}
	return firstResult(results, "move file")
}

// MoveFileAtomic updates a file's path and ensures destination folders exist in a single
// transaction. Post-commit steps (wiki-link raw_target recomputation, stem collision
// resolution) must be handled by the caller.
func (c *Client) MoveFileAtomic(ctx context.Context, vaultID, fileID, newPath string) (*models.File, error) {
	defer c.logOp(ctx, "file.move_atomic", time.Now())
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("move file atomic begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit

	// 1. Update file path + stem
	moveSQL := `
		UPDATE type::record("file", $id) SET
			path = $new_path,
			stem = $stem
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.File](ctx, tx, moveSQL, map[string]any{
		"id":       bareID("file", fileID),
		"new_path": newPath,
		"stem":     models.FilenameStem(newPath),
	})
	if err != nil {
		return nil, fmt.Errorf("move file atomic update: %w", err)
	}
	doc, err := firstResult(results, "move file atomic")
	if err != nil {
		return nil, fmt.Errorf("move file atomic: %w", err)
	}

	// 2. Ensure destination ancestor folders exist
	dir := path.Dir(newPath)
	if dir != "/" && dir != "." {
		folders := buildFolderRows(vaultID, dir)
		folderSQL := `INSERT INTO file $folders ON DUPLICATE KEY UPDATE id = id`
		if _, err := surrealdb.Query[any](ctx, tx, folderSQL, map[string]any{
			"folders": folders,
		}); err != nil {
			return nil, fmt.Errorf("move file atomic ensure folders: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("move file atomic commit: %w", err)
	}

	// Invalidate folder cache for the new directory
	c.InvalidateFolderCache(vaultID, dir)

	return doc, nil
}

// buildFolderRows returns folder row maps for a directory and all its ancestors,
// suitable for batch INSERT into the file table.
func buildFolderRows(vaultID, folderPath string) []map[string]any {
	var folders []string
	for cur := folderPath; cur != "/" && cur != "."; cur = path.Dir(cur) {
		folders = append(folders, cur)
	}
	vid := bareID("vault", vaultID)
	rows := make([]map[string]any, len(folders))
	for i, fp := range folders {
		rows[i] = map[string]any{
			"vault":     newRecordID("vault", vid),
			"path":      fp,
			"title":     path.Base(fp),
			"is_folder": true,
			"mime_type": "inode/directory",
			"size":      0,
			"labels":    []string{},
		}
	}
	return rows
}

// MoveByPrefixAtomic renames files, recomputes stems, moves folders, and ensures ancestor
// folders in a single transaction. Returns the count of moved non-folder files.
// Post-commit steps (wiki-link raw_target recomputation) must be handled by the caller.
func (c *Client) MoveByPrefixAtomic(ctx context.Context, vaultID, oldPrefix, newPrefix string) (int, error) {
	defer c.logOp(ctx, "file.move_by_prefix_atomic", time.Now())
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("move by prefix atomic begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit

	// 1. Move non-folder files
	moveSQL := `
		UPDATE file
		SET path = string::concat($new_prefix, string::slice(path, string::len($old_prefix)))
		WHERE vault = type::record("vault", $vault_id)
		  AND is_folder = false
		  AND string::starts_with(path, $old_prefix)
		RETURN AFTER
	`
	moveVars := map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_prefix": oldPrefix,
		"new_prefix": newPrefix,
	}
	moveResults, err := surrealdb.Query[[]models.File](ctx, tx, moveSQL, moveVars)
	if err != nil {
		return 0, fmt.Errorf("move by prefix atomic move files: %w", err)
	}
	count := countResults(moveResults)

	// 2. Recompute stems for all moved files using the results from step 1 (no re-query).
	//    Batched into a single multi-statement query within the interactive transaction.
	movedFiles := allResults(moveResults)
	if len(movedFiles) > 0 {
		var b strings.Builder
		vars := make(map[string]any, len(movedFiles)*2)
		for i, f := range movedFiles {
			idStr, err := models.RecordIDString(f.ID)
			if err != nil {
				return 0, fmt.Errorf("move by prefix atomic extract id: %w", err)
			}
			idKey := fmt.Sprintf("id_%d", i)
			stemKey := fmt.Sprintf("stem_%d", i)
			fmt.Fprintf(&b, "UPDATE type::record(\"file\", $%s) SET stem = $%s;\n", idKey, stemKey)
			vars[idKey] = idStr
			vars[stemKey] = models.FilenameStem(f.Path)
		}
		if _, err := surrealdb.Query[any](ctx, tx, b.String(), vars); err != nil {
			return 0, fmt.Errorf("move by prefix atomic recompute stems: %w", err)
		}
	}

	// 3. Move folder records
	oldFolderPath := strings.TrimSuffix(oldPrefix, "/")
	newFolderPath := strings.TrimSuffix(newPrefix, "/")
	oldFolderPrefix := oldFolderPath + "/"
	newFolderPrefix := newFolderPath + "/"
	folderVars := map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_path":   oldFolderPath,
		"new_path":   newFolderPath,
		"new_title":  path.Base(newFolderPath),
		"old_prefix": oldFolderPrefix,
		"new_prefix": newFolderPrefix,
	}
	if _, err := surrealdb.Query[any](ctx, tx,
		`UPDATE file SET path = $new_path, title = $new_title WHERE vault = type::record("vault", $vault_id) AND is_folder = true AND path = $old_path`, folderVars); err != nil {
		return 0, fmt.Errorf("move by prefix atomic move folder root: %w", err)
	}
	if _, err := surrealdb.Query[any](ctx, tx,
		`UPDATE file SET
			path = string::concat($new_prefix, string::slice(path, string::len($old_prefix))),
			title = array::last(string::split(string::concat($new_prefix, string::slice(path, string::len($old_prefix))), "/"))
		WHERE vault = type::record("vault", $vault_id)
		AND is_folder = true
		AND string::starts_with(path, $old_prefix)`, folderVars); err != nil {
		return 0, fmt.Errorf("move by prefix atomic move folder children: %w", err)
	}

	// 4. Ensure destination ancestor folders exist
	if newFolderPath != "/" && newFolderPath != "." {
		folders := buildFolderRows(vaultID, newFolderPath)
		if _, err := surrealdb.Query[any](ctx, tx,
			`INSERT INTO file $folders ON DUPLICATE KEY UPDATE id = id`,
			map[string]any{"folders": folders}); err != nil {
			return 0, fmt.Errorf("move by prefix atomic ensure folders: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("move by prefix atomic commit: %w", err)
	}

	// Invalidate folder cache for old and new paths
	c.InvalidateFolderCache(vaultID, oldFolderPath)
	c.InvalidateFolderCache(vaultID, newFolderPath)

	return count, nil
}

// ListUnprocessedFiles returns non-folder files that have a pending parse pipeline job.
// Used by ProcessAllPending for synchronous processing in tests and CLI commands.
// If vaultID is non-empty, only files in that vault are returned.
func (c *Client) ListUnprocessedFiles(ctx context.Context, vaultID string, limit int) ([]models.File, error) {
	defer c.logOp(ctx, "file.list_unprocessed", time.Now())
	if limit <= 0 {
		limit = 20
	}

	// Use a single query: join pipeline_job → file inline.
	// SurrealDB v3 requires ORDER BY fields to appear in the SELECT projection,
	// so we include created_at and extract the file fields via file.* aliasing.
	type jobWithFile struct {
		File models.File `json:"file"`
	}

	var sql string
	vars := map[string]any{"limit": limit}

	if vaultID != "" {
		sql = `
			SELECT file.*, created_at FROM pipeline_job
			WHERE type = 'parse' AND status IN ['pending', 'running']
			  AND file.vault = type::record("vault", $vault_id)
			ORDER BY created_at ASC
			LIMIT $limit
			FETCH file
		`
		vars["vault_id"] = bareID("vault", vaultID)
	} else {
		sql = `
			SELECT file.*, created_at FROM pipeline_job
			WHERE type = 'parse' AND status IN ['pending', 'running']
			ORDER BY created_at ASC
			LIMIT $limit
			FETCH file
		`
	}
	results, err := surrealdb.Query[[]jobWithFile](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list unprocessed: %w", err)
	}
	rows := allResults(results)
	seen := make(map[string]struct{}, len(rows))
	files := make([]models.File, 0, len(rows))
	for _, r := range rows {
		if r.File.ID.ID == nil {
			continue
		}
		key := fmt.Sprintf("%v", r.File.ID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		files = append(files, r.File)
	}
	return files, nil
}

// GetFilesByStem returns non-folder files matching a stem in a vault.
func (c *Client) GetFilesByStem(ctx context.Context, vaultID, stem string) ([]models.File, error) {
	defer c.logOp(ctx, "file.get_by_stem", time.Now())
	sql := `SELECT * FROM file WHERE vault = type::record("vault", $vault_id) AND stem = $stem AND is_folder = false`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"stem":     stem,
	})
	if err != nil {
		return nil, fmt.Errorf("get files by stem: %w", err)
	}
	return allResults(results), nil
}

// CountFilesByStem returns the count of non-folder files with the given stem in a vault.
func (c *Client) CountFilesByStem(ctx context.Context, vaultID, stem string) (int, error) {
	defer c.logOp(ctx, "file.count_by_stem", time.Now())
	sql := `SELECT count() AS total FROM file WHERE vault = type::record("vault", $vault_id) AND stem = $stem AND is_folder = false GROUP ALL`
	type countRow struct {
		Total int `json:"total"`
	}
	results, err := surrealdb.Query[[]countRow](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"stem":     stem,
	})
	if err != nil {
		return 0, fmt.Errorf("count files by stem: %w", err)
	}
	rows := allResults(results)
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].Total, nil
}

// ListFilesByPrefix returns non-folder files whose path starts with prefix in a vault.
func (c *Client) ListFilesByPrefix(ctx context.Context, vaultID, prefix string) ([]models.File, error) {
	defer c.logOp(ctx, "file.list_by_prefix", time.Now())
	sql := `SELECT * FROM file WHERE vault = type::record("vault", $vault_id) AND string::starts_with(path, $prefix) AND is_folder = false`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("list files by prefix: %w", err)
	}
	return allResults(results), nil
}

// ---------------------------------------------------------------------------
// Upsert (unified create-or-update for documents and binary files)
// ---------------------------------------------------------------------------

// UpsertFile creates or updates a file by vault+path.
// On update, previousFile contains the file state before the update (for versioning).
// Handles concurrent creates gracefully: if CreateFile hits a unique constraint
// violation (another request created the same path between our check and insert),
// we retry as an update.
//
// For binary files (non-empty Data), hash and size are computed from the data.
func (c *Client) UpsertFile(ctx context.Context, input models.FileInput) (file *models.File, created bool, previousFile *models.File, err error) {
	defer c.logOp(ctx, "file.upsert", time.Now())

	// For binary files, compute hash and size from the data.
	if len(input.Data) > 0 {
		h := sha256.Sum256(input.Data)
		hash := hex.EncodeToString(h[:])
		input.Hash = &hash
	}

	existing, err := c.GetFileByPath(ctx, input.VaultID, input.Path)
	if err != nil {
		return nil, false, nil, fmt.Errorf("check existing file: %w", err)
	}

	if existing == nil {
		file, err := c.CreateFile(ctx, input)
		if err != nil {
			if isUniqueViolation(err) {
				// Index says vault+path exists but GetFileByPath couldn't find it
				// (possible SurrealDB index/data inconsistency). Fall back to
				// UPDATE by vault+path. Return created=true so the caller
				// enqueues a pipeline job (we can't know if content changed).
				logutil.FromCtx(ctx).Warn("unique violation but file not found by path, falling back to update-by-path",
					"vault_id", input.VaultID, "path", input.Path, "error", err)
				file, err = c.updateFileByPath(ctx, input)
				if err != nil {
					return nil, false, nil, fmt.Errorf("upsert fallback update: %w", err)
				}
				return file, true, nil, nil
			}
			return nil, false, nil, fmt.Errorf("upsert create: %w", err)
		}
		return file, true, nil, nil
	}

	// Found existing file — update it
	idStr, err := models.RecordIDString(existing.ID)
	if err != nil {
		return nil, false, nil, fmt.Errorf("extract file id: %w", err)
	}

	file, err = c.UpdateFile(ctx, idStr, input)
	if err != nil {
		return nil, false, nil, err
	}
	return file, false, existing, nil
}

// ---------------------------------------------------------------------------
// Access tracking
// ---------------------------------------------------------------------------

// BatchUpdateFileAccess increments access_count and sets last_accessed_at for multiple files.
func (c *Client) BatchUpdateFileAccess(ctx context.Context, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	defer c.logOp(ctx, "file.batch_update_access", time.Now())
	records := make([]surrealmodels.RecordID, len(fileIDs))
	for i, id := range fileIDs {
		records[i] = newRecordID("file", id)
	}
	sql := `UPDATE file SET last_accessed_at = time::now(), access_count += 1 WHERE id IN $ids`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"ids": records}); err != nil {
		return fmt.Errorf("batch update file access: %w", err)
	}
	return nil
}

// SetFileAccessCount sets access_count to a specific value and updates last_accessed_at.
func (c *Client) SetFileAccessCount(ctx context.Context, fileID string, count int) error {
	defer c.logOp(ctx, "file.set_access_count", time.Now())
	sql := `UPDATE type::record("file", $id) SET last_accessed_at = time::now(), access_count = $count`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{"id": fileID, "count": count}); err != nil {
		return fmt.Errorf("set file access count: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Labels
// ---------------------------------------------------------------------------

// ListLabels returns all distinct labels used by files in the given vault.
func (c *Client) ListLabels(ctx context.Context, vaultID string) ([]string, error) {
	defer c.logOp(ctx, "file.list_labels", time.Now())
	sql := `RETURN array::distinct(array::flatten(
		(SELECT labels FROM file WHERE vault = type::record("vault", $vault_id)).labels
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

// ListLabelsWithCounts returns labels with their file counts for the given vault.
func (c *Client) ListLabelsWithCounts(ctx context.Context, vaultID string) ([]models.LabelCount, error) {
	defer c.logOp(ctx, "file.list_labels_with_counts", time.Now())
	sql := `SELECT label, count() AS count FROM (SELECT labels AS label FROM file WHERE vault = type::record("vault", $vault_id) AND array::len(labels) > 0 SPLIT labels) GROUP BY label ORDER BY count DESC`
	results, err := surrealdb.Query[[]models.LabelCount](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("list labels with counts: %w", err)
	}
	if results == nil || len(*results) == 0 {
		return []models.LabelCount{}, nil
	}
	counts := (*results)[0].Result
	if counts == nil {
		return []models.LabelCount{}, nil
	}
	return counts, nil
}

// GetFilesByAllLabels returns files in a vault that have ALL of the specified labels
// (AND filter). Uses graph traversal through has_label edges.
func (c *Client) GetFilesByAllLabels(ctx context.Context, vaultID string, labels []string) ([]models.File, error) {
	defer c.logOp(ctx, "file.get_by_all_labels", time.Now())
	if len(labels) == 0 {
		return nil, nil
	}

	// Build conditions for each label: count(->has_label->label[WHERE name = $l0]) > 0 AND ...
	var conditions []string
	vars := map[string]any{
		"vault_id": bareID("vault", vaultID),
	}
	for i, l := range labels {
		paramName := fmt.Sprintf("l%d", i)
		conditions = append(conditions, fmt.Sprintf(`count(->has_label->label[WHERE name = $%s]) > 0`, paramName))
		vars[paramName] = strings.ToLower(strings.TrimSpace(l))
	}

	sql := fmt.Sprintf(`SELECT * FROM file WHERE vault = type::record("vault", $vault_id) AND %s`,
		strings.Join(conditions, " AND "))

	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("get files by all labels: %w", err)
	}
	return allResults(results), nil
}

// ---------------------------------------------------------------------------
// Metadata projections
// ---------------------------------------------------------------------------

// GetFileMetaByPath returns lightweight metadata for a file (no content/data).
// Returns nil if the file doesn't exist.
func (c *Client) GetFileMetaByPath(ctx context.Context, vaultID, filePath string) (*models.FileMeta, error) {
	defer c.logOp(ctx, "file.get_meta_by_path", time.Now())
	sql := `SELECT path, mime_type, size, hash ?? null AS hash, is_folder, updated_at FROM file WHERE vault = type::record("vault", $vault_id) AND path = $path LIMIT 1`
	results, err := surrealdb.Query[[]models.FileMeta](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     filePath,
	})
	if err != nil {
		return nil, fmt.Errorf("get file meta by path: %w", err)
	}
	return firstResultOpt(results), nil
}

// GetFileMetaByPaths returns lightweight metadata for multiple files in a single query.
// Returns a map of path → *FileMeta. Missing files are absent from the map.
func (c *Client) GetFileMetaByPaths(ctx context.Context, vaultID string, paths []string) (map[string]*models.FileMeta, error) {
	defer c.logOp(ctx, "file.get_meta_by_paths", time.Now())
	if len(paths) == 0 {
		return map[string]*models.FileMeta{}, nil
	}
	sql := `SELECT path, mime_type, size, hash ?? null AS hash, is_folder, updated_at FROM file WHERE vault = type::record("vault", $vault_id) AND path IN $paths`
	results, err := surrealdb.Query[[]models.FileMeta](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"paths":    paths,
	})
	if err != nil {
		return nil, fmt.Errorf("get file meta by paths: %w", err)
	}
	all := allResults(results)
	m := make(map[string]*models.FileMeta, len(all))
	for i := range all {
		m[all[i].Path] = &all[i]
	}
	return m, nil
}

// ListFileMetas returns lightweight metadata (no content/data) for files matching the filter.
func (c *Client) ListFileMetas(ctx context.Context, filter ListFilesFilter) ([]models.FileMeta, error) {
	defer c.logOp(ctx, "file.list_metas", time.Now())
	where, vars, suffix, err := buildFileFilter(filter)
	if err != nil {
		return nil, fmt.Errorf("list file metas: %w", err)
	}
	sql := fmt.Sprintf("SELECT path, title, mime_type, size, hash ?? null AS hash, is_folder, updated_at FROM file WHERE %s %s", where, suffix)

	results, err := surrealdb.Query[[]models.FileMeta](ctx, c.DB(), sql, vars)
	if err != nil {
		return nil, fmt.Errorf("list file metas: %w", err)
	}
	return allResults(results), nil
}

// ---------------------------------------------------------------------------
// Folder operations (files with is_folder = true)
// ---------------------------------------------------------------------------

// CreateFolder creates a single folder record (file with is_folder=true).
// Returns the created folder.
func (c *Client) CreateFolder(ctx context.Context, vaultID, folderPath string) (*models.File, error) {
	defer c.logOp(ctx, "file.create_folder", time.Now())

	sql := `
		INSERT INTO file {
			vault: type::record("vault", $vault_id),
			path: $path,
			title: $title,
			is_folder: true,
			mime_type: "inode/directory",
			size: 0,
			labels: [],
		} ON DUPLICATE KEY UPDATE id = id
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     folderPath,
		"title":    path.Base(folderPath),
	})
	if err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}
	return firstResult(results, "create folder")
}

// EnsureFolders idempotently creates all ancestor folders for a file path,
// caching recent calls to skip redundant DB round-trips (60s TTL).
// For example, given "/guides/sub/file.md", it creates "/guides" and "/guides/sub".
func (c *Client) EnsureFolders(ctx context.Context, vaultID, filePath string) error {
	defer c.logOp(ctx, "file.ensure_folders", time.Now())
	filePath = models.NormalizePath(filePath)
	dir := path.Dir(filePath)
	if dir == "/" || dir == "." {
		return nil // root-level file, no folders to create
	}

	// Check cache — skip DB call if this folder was recently ensured
	cacheKey := vaultID + ":" + dir
	if expiry, ok := c.folderCache.Load(cacheKey); ok {
		if t, ok := expiry.(time.Time); ok && t.After(time.Now()) {
			return nil
		}
		c.folderCache.Delete(cacheKey)
	}

	if err := c.EnsureFolderPath(ctx, vaultID, dir); err != nil {
		return fmt.Errorf("ensure folders for %s: %w", filePath, err)
	}

	// Cache for 60 seconds
	c.folderCache.Store(cacheKey, time.Now().Add(60*time.Second))
	return nil
}

// EnsureFolderPath idempotently creates a folder and all its ancestors.
// For example, given "/guides/sub", it creates "/guides" and "/guides/sub".
// All folders are inserted in a single batched query to avoid N+1 round-trips.
func (c *Client) EnsureFolderPath(ctx context.Context, vaultID, folderPath string) error {
	defer c.logOp(ctx, "file.ensure_folder_path", time.Now())
	folderPath = models.NormalizePath(folderPath)
	if folderPath == "/" || folderPath == "." {
		return nil
	}

	rows := buildFolderRows(vaultID, folderPath)
	sql := `INSERT INTO file $folders ON DUPLICATE KEY UPDATE id = id`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"folders": rows,
	}); err != nil {
		return fmt.Errorf("ensure folder path %q: %w", folderPath, err)
	}

	return nil
}

// ListFolders returns all folders in a vault, ordered by path.
func (c *Client) ListFolders(ctx context.Context, vaultID string) ([]models.File, error) {
	defer c.logOp(ctx, "file.list_folders", time.Now())
	sql := `SELECT * FROM file WHERE vault = type::record("vault", $vault_id) AND is_folder = true ORDER BY path ASC`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return allResults(results), nil
}

// ListChildFolders returns immediate child folders of parentPath in a vault.
// Only folders whose path starts with parentPath+"/" are fetched from the DB,
// then filtered in Go to exclude nested descendants (much faster than loading all folders).
func (c *Client) ListChildFolders(ctx context.Context, vaultID, parentPath string) ([]models.File, error) {
	defer c.logOp(ctx, "file.list_child_folders", time.Now())
	prefix := parentPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	sql := `SELECT * FROM file WHERE vault = type::record("vault", $vault_id) AND is_folder = true AND string::starts_with(path, $prefix) ORDER BY path ASC`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   prefix,
	})
	if err != nil {
		return nil, fmt.Errorf("list child folders: %w", err)
	}
	all := allResults(results)
	if all == nil {
		return nil, nil
	}

	// Filter to immediate children only (no additional "/" after prefix)
	var children []models.File
	for _, f := range all {
		rel := strings.TrimPrefix(f.Path, prefix)
		if rel != "" && !strings.Contains(rel, "/") {
			children = append(children, f)
		}
	}
	return children, nil
}

// GetFolderByPath returns a single folder by vault and path, or nil if not found.
func (c *Client) GetFolderByPath(ctx context.Context, vaultID, folderPath string) (*models.File, error) {
	defer c.logOp(ctx, "file.get_folder_by_path", time.Now())
	sql := `SELECT * FROM file WHERE vault = type::record("vault", $vault_id) AND is_folder = true AND path = $path LIMIT 1`
	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     folderPath,
	})
	if err != nil {
		return nil, fmt.Errorf("get folder by path: %w", err)
	}
	return firstResultOpt(results), nil
}

// DeleteFolder deletes a single folder and all its children (paths starting with folderPath + "/").
func (c *Client) DeleteFolder(ctx context.Context, vaultID, folderPath string) error {
	defer c.logOp(ctx, "file.delete_folder", time.Now())
	if _, err := c.deleteFolderTree(ctx, vaultID, folderPath); err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}
	return nil
}

// DeleteFoldersByPrefix deletes all folders whose path starts with the given prefix
// or matches the prefix itself (without trailing slash). Returns the number of deleted folders.
func (c *Client) DeleteFoldersByPrefix(ctx context.Context, vaultID, prefix string) (int, error) {
	defer c.logOp(ctx, "file.delete_folders_by_prefix", time.Now())
	folderPath := strings.TrimSuffix(prefix, "/")
	return c.deleteFolderTree(ctx, vaultID, folderPath)
}

// InvalidateFolderCache removes cached folder entries for a vault+path prefix.
// Scans all cached entries (expected to be small — one entry per recently-used folder).
func (c *Client) InvalidateFolderCache(vaultID, folderPath string) {
	prefix := vaultID + ":" + folderPath
	c.folderCache.Range(func(key, _ any) bool {
		k, ok := key.(string)
		if !ok {
			return true
		}
		if k == prefix || strings.HasPrefix(k, prefix+"/") {
			c.folderCache.Delete(key)
		}
		return true
	})
}

// deleteFolderTree deletes a folder and all its children in a transaction.
// Two statements are needed because SurrealDB v3 doesn't support parenthesized OR in WHERE.
// Returns the total number of deleted folders.
func (c *Client) deleteFolderTree(ctx context.Context, vaultID, folderPath string) (int, error) {
	defer c.logOp(ctx, "file.delete_folder_tree", time.Now())
	c.InvalidateFolderCache(vaultID, folderPath)
	prefix := folderPath + "/"
	vars := map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     folderPath,
		"prefix":   prefix,
	}

	tx, err := c.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete folder tree begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit; rollback failure is unrecoverable

	r1, err := surrealdb.Query[[]models.File](ctx, tx,
		`DELETE FROM file WHERE vault = type::record("vault", $vault_id) AND is_folder = true AND path = $path RETURN BEFORE`, vars)
	if err != nil {
		return 0, fmt.Errorf("delete folder tree %q: %w", folderPath, err)
	}
	r2, err := surrealdb.Query[[]models.File](ctx, tx,
		`DELETE FROM file WHERE vault = type::record("vault", $vault_id) AND is_folder = true AND string::starts_with(path, $prefix) RETURN BEFORE`, vars)
	if err != nil {
		return 0, fmt.Errorf("delete folder tree children %q: %w", folderPath, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("delete folder tree commit: %w", err)
	}

	return countResults(r1) + countResults(r2), nil
}

// MoveFoldersByPrefix renames all folders whose path starts with oldPrefix,
// replacing oldPrefix with newPrefix. Also updates the title field.
// Accepts folder paths (e.g. "/guides", "/docs") — trailing slashes are handled internally.
// Returns the number of moved folders.
func (c *Client) MoveFoldersByPrefix(ctx context.Context, vaultID, oldPath, newPath string) (int, error) {
	defer c.logOp(ctx, "file.move_folders_by_prefix", time.Now())
	oldFolderPath := strings.TrimSuffix(oldPath, "/")
	newFolderPath := strings.TrimSuffix(newPath, "/")

	c.InvalidateFolderCache(vaultID, oldFolderPath)
	c.InvalidateFolderCache(vaultID, newFolderPath)

	oldPrefix := oldFolderPath + "/"
	newPrefix := newFolderPath + "/"

	vars := map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_path":   oldFolderPath,
		"new_path":   newFolderPath,
		"new_title":  path.Base(newFolderPath),
		"old_prefix": oldPrefix,
		"new_prefix": newPrefix,
	}

	tx, err := c.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("move folders begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit; rollback failure is unrecoverable

	r1, err := surrealdb.Query[[]models.File](ctx, tx,
		`UPDATE file SET path = $new_path, title = $new_title WHERE vault = type::record("vault", $vault_id) AND is_folder = true AND path = $old_path RETURN BEFORE`, vars)
	if err != nil {
		return 0, fmt.Errorf("move folder root: %w", err)
	}
	r2, err := surrealdb.Query[[]models.File](ctx, tx,
		`UPDATE file SET
			path = string::concat($new_prefix, string::slice(path, string::len($old_prefix))),
			title = array::last(string::split(string::concat($new_prefix, string::slice(path, string::len($old_prefix))), "/"))
		WHERE vault = type::record("vault", $vault_id)
		AND is_folder = true
		AND string::starts_with(path, $old_prefix)
		RETURN BEFORE`, vars)
	if err != nil {
		return 0, fmt.Errorf("move folder children: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("move folders commit: %w", err)
	}

	return countResults(r1) + countResults(r2), nil
}

// ---------------------------------------------------------------------------
// Folder embedding control
// ---------------------------------------------------------------------------

// SetFolderNoEmbed sets the no_embed flag on a folder record.
func (c *Client) SetFolderNoEmbed(ctx context.Context, vaultID, folderPath string, noEmbed bool) error {
	defer c.logOp(ctx, "file.set_folder_no_embed", time.Now())
	sql := `UPDATE file SET no_embed = $no_embed WHERE vault = type::record("vault", $vault_id) AND is_folder = true AND path = $path`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"path":     folderPath,
		"no_embed": noEmbed,
	}); err != nil {
		return fmt.Errorf("set folder no_embed: %w", err)
	}
	return nil
}

// IsPathNoEmbed checks whether any ancestor folder of filePath has no_embed = true.
// Returns false for root-level files (no ancestors to check).
func (c *Client) IsPathNoEmbed(ctx context.Context, vaultID, filePath string) (bool, error) {
	defer c.logOp(ctx, "file.is_path_no_embed", time.Now())

	// Collect ancestor folder paths.
	filePath = path.Clean(filePath)
	var ancestors []string
	for dir := path.Dir(filePath); dir != "/" && dir != "."; dir = path.Dir(dir) {
		ancestors = append(ancestors, dir)
	}
	if len(ancestors) == 0 {
		return false, nil
	}

	type countRow struct {
		Total int `json:"total"`
	}
	sql := `SELECT count() AS total FROM file WHERE vault = type::record("vault", $vault_id) AND is_folder = true AND no_embed = true AND path IN $ancestors GROUP ALL`
	results, err := surrealdb.Query[[]countRow](ctx, c.DB(), sql, map[string]any{
		"vault_id":  bareID("vault", vaultID),
		"ancestors": ancestors,
	})
	if err != nil {
		return false, fmt.Errorf("check path no_embed: %w", err)
	}
	rows := allResults(results)
	return len(rows) > 0 && rows[0].Total > 0, nil
}

// ClearFileHashes sets hash = NONE for all files, optionally filtered by vault.
// Returns the number of updated files.
func (c *Client) ClearFileHashes(ctx context.Context, vaultID string) (int, error) {
	defer c.logOp(ctx, "file.clear_hashes", time.Now())
	var sql string
	vars := map[string]any{}

	if vaultID != "" {
		sql = `UPDATE file SET hash = NONE WHERE vault = type::record("vault", $vault_id) AND is_folder = false RETURN AFTER`
		vars["vault_id"] = bareID("vault", vaultID)
	} else {
		sql = `UPDATE file SET hash = NONE WHERE is_folder = false RETURN AFTER`
	}

	results, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, vars)
	if err != nil {
		return 0, fmt.Errorf("clear file hashes: %w", err)
	}
	return countResults(results), nil
}

// SetFileDirtyTasks sets or clears the dirty_tasks flag on a file.
func (c *Client) SetFileDirtyTasks(ctx context.Context, fileID string, dirty bool) error {
	defer c.logOp(ctx, "file.set_dirty_tasks", time.Now())

	sql := `UPDATE type::record("file", $id) SET dirty_tasks = $dirty`
	_, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"id":    bareID("file", fileID),
		"dirty": dirty,
	})
	if err != nil {
		return fmt.Errorf("set dirty tasks: %w", err)
	}
	return nil
}

// UpdateFileHash updates the content hash on a file record.
func (c *Client) UpdateFileHash(ctx context.Context, fileID string, hash *string) error {
	defer c.logOp(ctx, "file.update_hash", time.Now())

	sql := `UPDATE type::record("file", $id) SET hash = $hash`
	_, err := surrealdb.Query[[]models.File](ctx, c.DB(), sql, map[string]any{
		"id":   bareID("file", fileID),
		"hash": optionalString(hash),
	})
	if err != nil {
		return fmt.Errorf("update file hash: %w", err)
	}
	return nil
}
