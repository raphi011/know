package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func (c *Client) CreateVault(ctx context.Context, userID string, input models.VaultInput) (*models.Vault, error) {
	defer c.logOp(ctx, "vault.create", time.Now())
	sql := `
		CREATE vault SET
			name = $name,
			description = $description,
			created_by = type::record("user", $user_id)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"name":        input.Name,
		"description": optionalString(input.Description),
		"user_id":     userID,
	})
	if err != nil {
		return nil, fmt.Errorf("create vault: %w", err)
	}
	return firstResult(results, "create vault")
}

func (c *Client) CreateVaultWithID(ctx context.Context, vaultID string, userID string, input models.VaultInput) (*models.Vault, error) {
	defer c.logOp(ctx, "vault.create_with_id", time.Now())
	sql := `
		CREATE type::record("vault", $vault_id) SET
			name = $name,
			description = $description,
			created_by = type::record("user", $user_id)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"vault_id":    bareID("vault", vaultID),
		"name":        input.Name,
		"description": optionalString(input.Description),
		"user_id":     userID,
	})
	if err != nil {
		return nil, fmt.Errorf("create vault with id: %w", err)
	}
	return firstResult(results, "create vault with id")
}

// CreateVaultWithOwner creates a vault owned by a specific user (private vault).
func (c *Client) CreateVaultWithOwner(ctx context.Context, userID string, input models.VaultInput) (*models.Vault, error) {
	defer c.logOp(ctx, "vault.create_with_owner", time.Now())
	sql := `
		CREATE vault SET
			name = $name,
			description = $description,
			owner = type::record("user", $user_id),
			created_by = type::record("user", $user_id)
		RETURN AFTER
	`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"name":        input.Name,
		"description": optionalString(input.Description),
		"user_id":     bareID("user", userID),
	})
	if err != nil {
		return nil, fmt.Errorf("create vault with owner: %w", err)
	}
	return firstResult(results, "create vault with owner")
}

// GetVaultByOwner returns the private vault owned by the given user.
func (c *Client) GetVaultByOwner(ctx context.Context, userID string) (*models.Vault, error) {
	defer c.logOp(ctx, "vault.get_by_owner", time.Now())
	sql := `SELECT * FROM vault WHERE owner = type::record("user", $user_id) LIMIT 1`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"user_id": bareID("user", userID),
	})
	if err != nil {
		return nil, fmt.Errorf("get vault by owner: %w", err)
	}
	return firstResultOpt(results), nil
}

func (c *Client) GetVault(ctx context.Context, id string) (*models.Vault, error) {
	defer c.logOp(ctx, "vault.get", time.Now())
	sql := `SELECT * FROM type::record("vault", $id)`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"id": id,
	})
	if err != nil {
		return nil, fmt.Errorf("get vault: %w", err)
	}
	return firstResultOpt(results), nil
}

func (c *Client) GetVaultByName(ctx context.Context, name string) (*models.Vault, error) {
	defer c.logOp(ctx, "vault.get_by_name", time.Now())
	sql := `SELECT * FROM vault WHERE name = $name LIMIT 1`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"name": name,
	})
	if err != nil {
		return nil, fmt.Errorf("get vault by name: %w", err)
	}
	return firstResultOpt(results), nil
}

func (c *Client) ListVaults(ctx context.Context) ([]models.Vault, error) {
	defer c.logOp(ctx, "vault.list", time.Now())
	sql := `SELECT * FROM vault ORDER BY name ASC`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, nil)
	if err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}
	return allResults(results), nil
}

func (c *Client) DeleteVault(ctx context.Context, id string) error {
	defer c.logOp(ctx, "vault.delete", time.Now())
	tx, err := c.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete vault begin tx: %w", err)
	}
	defer tx.Cancel(ctx) //nolint:errcheck // no-op after Commit; rollback failure is unrecoverable

	// Delete all vault data, then the vault itself.
	// Document cascade events (ASYNC) handle chunks, wiki_links, and relations after commit.
	vars := map[string]any{"id": id}
	for _, t := range []struct {
		name string
		sql  string
	}{
		{"file", `DELETE FROM file WHERE vault = type::record("vault", $id)`},
		{"vault", `DELETE type::record("vault", $id)`},
	} {
		if _, err := surrealdb.Query[any](ctx, tx, t.sql, vars); err != nil {
			return fmt.Errorf("delete vault %s: %w", t.name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("delete vault commit: %w", err)
	}
	return nil
}

// UpdateVaultSettings replaces the vault's settings with the provided value.
// The caller is responsible for merging defaults before calling this.
func (c *Client) UpdateVaultSettings(ctx context.Context, vaultID string, settings models.VaultSettings) (*models.Vault, error) {
	defer c.logOp(ctx, "vault.update_settings", time.Now())
	sql := `UPDATE type::record("vault", $id) SET settings = $settings RETURN AFTER`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"id":       bareID("vault", vaultID),
		"settings": settings,
	})
	if err != nil {
		return nil, fmt.Errorf("update vault settings: %w", err)
	}
	return firstResult(results, "update vault settings")
}

// GetVaultsByIDs fetches multiple vaults by their IDs in a single query.
func (c *Client) GetVaultsByIDs(ctx context.Context, ids []string) ([]models.Vault, error) {
	defer c.logOp(ctx, "vault.get_by_ids", time.Now())
	if len(ids) == 0 {
		return nil, nil
	}
	records := make([]surrealmodels.RecordID, len(ids))
	for i, id := range ids {
		records[i] = newRecordID("vault", bareID("vault", id))
	}
	sql := `SELECT * FROM vault WHERE id IN $ids`
	results, err := surrealdb.Query[[]models.Vault](ctx, c.DB(), sql, map[string]any{
		"ids": records,
	})
	if err != nil {
		return nil, fmt.Errorf("get vaults by ids: %w", err)
	}
	return allResults(results), nil
}

// ListDocumentPaths returns all non-folder file paths in a vault (for folder derivation).
func (c *Client) ListDocumentPaths(ctx context.Context, vaultID string) ([]string, error) {
	defer c.logOp(ctx, "vault.list_document_paths", time.Now())
	sql := `SELECT path FROM file WHERE vault = type::record("vault", $vault_id) AND is_folder = false`
	results, err := surrealdb.Query[[]struct {
		Path string `json:"path"`
	}](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
	})
	if err != nil {
		return nil, fmt.Errorf("list document paths: %w", err)
	}
	rows := allResults(results)
	if rows == nil {
		return nil, nil
	}
	paths := make([]string, len(rows))
	for i, r := range rows {
		paths[i] = r.Path
	}
	return paths, nil
}

// VaultInfoStats holds aggregated vault statistics from a batch query.
type VaultInfoStats struct {
	DocumentCount      int
	UnprocessedDocs    int
	ChunkTotal         int
	ChunkWithEmbedding int
	ChunkPending       int
	LabelCount         int
	TopLabels          []models.LabelStat
	Members            []models.MemberStat
	AssetCount         int
	AssetTotalSize     int64
	WikiLinkTotal      int
	WikiLinkBroken     int
	VersionCount       int
	ConversationCount  int
	TokenInput         int64
	TokenOutput        int64
}

// GetVaultInfo retrieves comprehensive stats about a vault in a single batch query.
func (c *Client) GetVaultInfo(ctx context.Context, vaultID string) (*VaultInfoStats, error) {
	defer c.logOp(ctx, "vault.get_info", time.Now())

	vid := bareID("vault", vaultID)

	// 10-statement batch query — one round-trip
	sql := `
		-- 0: document count
		SELECT count() AS total
			FROM file WHERE vault = type::record("vault", $vid) AND is_folder = false GROUP ALL;

		-- 1: pending pipeline jobs
		SELECT count() AS total
			FROM pipeline_job WHERE file.vault = type::record("vault", $vid) AND status IN ['pending', 'running'] GROUP ALL;

		-- 2: chunk stats (join through file)
		SELECT count() AS total,
			   math::sum(IF embedding IS NOT NONE THEN 1 ELSE 0 END) AS embedded,
			   math::sum(IF embedding IS NONE THEN 1 ELSE 0 END) AS pending
			FROM chunk WHERE file.vault = type::record("vault", $vid) GROUP ALL;

		-- 3: label count
		SELECT count() AS total FROM label WHERE vault = type::record("vault", $vid) GROUP ALL;

		-- 4: top 10 labels by document count
		SELECT out.name AS name, count() AS count
			FROM has_label WHERE out.vault = type::record("vault", $vid)
			GROUP BY name ORDER BY count DESC LIMIT 10;

		-- 5: members + roles
		SELECT user.name AS name, role FROM vault_member WHERE vault = type::record("vault", $vid);

		-- 6: asset count + total size
		SELECT count() AS total, math::sum(size) AS total_size
			FROM file WHERE vault = type::record("vault", $vid) AND is_folder = false AND data IS NOT NONE GROUP ALL;

		-- 7: wiki-link total + broken
		SELECT count() AS total, math::sum(IF to_file IS NONE THEN 1 ELSE 0 END) AS broken
			FROM wiki_link WHERE vault = type::record("vault", $vid) GROUP ALL;

		-- 8: version count
		SELECT count() AS total FROM file_version WHERE vault = type::record("vault", $vid) GROUP ALL;

		-- 9: conversation count + tokens
		SELECT count() AS total, math::sum(token_input) AS token_input, math::sum(token_output) AS token_output
			FROM conversation WHERE vault = type::record("vault", $vid) GROUP ALL;
	`

	results, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"vid": vid,
	})
	if err != nil {
		return nil, fmt.Errorf("get vault info: %w", err)
	}
	if results == nil {
		return nil, fmt.Errorf("get vault info: nil results")
	}
	if len(*results) < 10 {
		return nil, fmt.Errorf("get vault info: expected 10 results, got %d", len(*results))
	}

	stats := &VaultInfoStats{}

	// Helper: marshal result to JSON and unmarshal into target slice
	decodeResult := func(idx int, target any) error {
		raw, err := json.Marshal((*results)[idx].Result)
		if err != nil {
			return fmt.Errorf("marshal result %d: %w", idx, err)
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("unmarshal result %d: %w", idx, err)
		}
		return nil
	}

	// Helper for single-row GROUP ALL results with just a total field.
	decodeTotalCount := func(idx int) (int, error) {
		var rows []struct {
			Total int `json:"total"`
		}
		if err := decodeResult(idx, &rows); err != nil {
			return 0, err
		}
		if len(rows) > 0 {
			return rows[0].Total, nil
		}
		return 0, nil
	}

	// 0: document count
	docCount, err := decodeTotalCount(0)
	if err != nil {
		return nil, fmt.Errorf("decode doc count: %w", err)
	}
	stats.DocumentCount = docCount

	// 1: unprocessed (pending pipeline jobs)
	unprocessed, err := decodeTotalCount(1)
	if err != nil {
		return nil, fmt.Errorf("decode unprocessed count: %w", err)
	}
	stats.UnprocessedDocs = unprocessed

	// 2: chunks
	var chunkStats []struct {
		Total    int `json:"total"`
		Embedded int `json:"embedded"`
		Pending  int `json:"pending"`
	}
	if err := decodeResult(2, &chunkStats); err != nil {
		return nil, fmt.Errorf("decode chunk stats: %w", err)
	}
	if len(chunkStats) > 0 {
		stats.ChunkTotal = chunkStats[0].Total
		stats.ChunkWithEmbedding = chunkStats[0].Embedded
		stats.ChunkPending = chunkStats[0].Pending
	}

	// 3: label count
	labelCount, err := decodeTotalCount(3)
	if err != nil {
		return nil, fmt.Errorf("decode label count: %w", err)
	}
	stats.LabelCount = labelCount

	// 4: top labels
	var topLabels []models.LabelStat
	if err := decodeResult(4, &topLabels); err != nil {
		return nil, fmt.Errorf("decode top labels: %w", err)
	}
	if topLabels == nil {
		topLabels = []models.LabelStat{}
	}
	stats.TopLabels = topLabels

	// 5: members
	var members []models.MemberStat
	if err := decodeResult(5, &members); err != nil {
		return nil, fmt.Errorf("decode members: %w", err)
	}
	if members == nil {
		members = []models.MemberStat{}
	}
	stats.Members = members

	// 6: assets
	var assetStats []struct {
		Total     int   `json:"total"`
		TotalSize int64 `json:"total_size"`
	}
	if err := decodeResult(6, &assetStats); err != nil {
		return nil, fmt.Errorf("decode asset stats: %w", err)
	}
	if len(assetStats) > 0 {
		stats.AssetCount = assetStats[0].Total
		stats.AssetTotalSize = assetStats[0].TotalSize
	}

	// 7: wiki-links
	var wikiStats []struct {
		Total  int `json:"total"`
		Broken int `json:"broken"`
	}
	if err := decodeResult(7, &wikiStats); err != nil {
		return nil, fmt.Errorf("decode wiki-link stats: %w", err)
	}
	if len(wikiStats) > 0 {
		stats.WikiLinkTotal = wikiStats[0].Total
		stats.WikiLinkBroken = wikiStats[0].Broken
	}

	// 8: versions
	versionCount, err := decodeTotalCount(8)
	if err != nil {
		return nil, fmt.Errorf("decode version count: %w", err)
	}
	stats.VersionCount = versionCount

	// 9: conversations
	var convStats []struct {
		Total       int   `json:"total"`
		TokenInput  int64 `json:"token_input"`
		TokenOutput int64 `json:"token_output"`
	}
	if err := decodeResult(9, &convStats); err != nil {
		return nil, fmt.Errorf("decode conversation stats: %w", err)
	}
	if len(convStats) > 0 {
		stats.ConversationCount = convStats[0].Total
		stats.TokenInput = convStats[0].TokenInput
		stats.TokenOutput = convStats[0].TokenOutput
	}

	return stats, nil
}
