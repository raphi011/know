package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// ListAllTokens returns all API tokens across all users.
func (c *Client) ListAllTokens(ctx context.Context) ([]models.APIToken, error) {
	defer c.logOp(ctx, "token.list_all", time.Now())
	sql := `SELECT * FROM api_token ORDER BY created_at DESC`
	results, err := surrealdb.Query[[]models.APIToken](ctx, c.DB(), sql, nil)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	return allResults(results), nil
}

// ListAllVaultMembers returns all vault memberships across all users and vaults.
func (c *Client) ListAllVaultMembers(ctx context.Context) ([]models.VaultMember, error) {
	defer c.logOp(ctx, "vault_member.list_all", time.Now())
	sql := `SELECT * FROM vault_member ORDER BY created_at DESC`
	results, err := surrealdb.Query[[]models.VaultMember](ctx, c.DB(), sql, nil)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	return allResults(results), nil
}

// allTables returns all table names defined in the schema, in reverse dependency order
// (safe for REMOVE TABLE).
func allTables() []string {
	return []string{
		"bookmark", "oauth_client", "oauth_auth_code", "device_code",
		"pipeline_job", "agent_checkpoint", "remote", "task",
		"file_tombstone", "search_query", "message", "conversation",
		"vault_member", "api_token", "has_label", "label",
		"file_relation", "external_link", "wiki_link", "chunk",
		"file_version", "file", "vault", "user",
	}
}

// ResetSchema drops all tables and the analyzer, then re-applies the schema.
// This destroys ALL data — callers must snapshot identity data first.
func (c *Client) ResetSchema(ctx context.Context, embedDimension int) error {
	defer c.logOp(ctx, "schema.reset", time.Now())
	logger := logutil.FromCtx(ctx)
	logger.Warn("resetting database schema (REMOVE all tables + analyzer)")

	for _, t := range allTables() {
		sql := fmt.Sprintf("REMOVE TABLE IF EXISTS %s", t)
		if _, err := surrealdb.Query[any](ctx, c.DB(), sql, nil); err != nil {
			return fmt.Errorf("remove table %s: %w", t, err)
		}
	}

	if _, err := surrealdb.Query[any](ctx, c.DB(), "REMOVE ANALYZER IF EXISTS know_analyzer", nil); err != nil {
		return fmt.Errorf("remove analyzer: %w", err)
	}

	logger.Info("all tables and analyzer removed, re-applying schema")
	return c.InitSchema(ctx, embedDimension)
}

// IdentitySnapshot holds the identity data to preserve across a schema reset.
// Restore order matters: users → vaults → vault_members → tokens (due to foreign keys).
type IdentitySnapshot struct {
	Users        []models.User
	Vaults       []models.Vault
	VaultMembers []models.VaultMember
	Tokens       []models.APIToken
}

// SnapshotIdentity reads all users, vaults, memberships, and tokens from the database.
func (c *Client) SnapshotIdentity(ctx context.Context) (*IdentitySnapshot, error) {
	defer c.logOp(ctx, "identity.snapshot", time.Now())

	users, err := c.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot users: %w", err)
	}

	vaults, err := c.ListVaults(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot vaults: %w", err)
	}

	members, err := c.ListAllVaultMembers(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot vault members: %w", err)
	}

	tokens, err := c.ListAllTokens(ctx)
	if err != nil {
		return nil, fmt.Errorf("snapshot tokens: %w", err)
	}

	return &IdentitySnapshot{
		Users:        users,
		Vaults:       vaults,
		VaultMembers: members,
		Tokens:       tokens,
	}, nil
}

// RestoreIdentity re-inserts snapshotted identity data after a schema reset.
// Uses a single transaction so partial failures roll back cleanly.
func (c *Client) RestoreIdentity(ctx context.Context, snap *IdentitySnapshot) error {
	defer c.logOp(ctx, "identity.restore", time.Now())
	logger := logutil.FromCtx(ctx)

	// Build a single transactional query to restore all identity data atomically.
	var b strings.Builder
	params := map[string]any{}

	b.WriteString("BEGIN TRANSACTION;\n")

	for i, u := range snap.Users {
		userID, err := models.RecordIDString(u.ID)
		if err != nil {
			return fmt.Errorf("restore user id: %w", err)
		}
		prefix := fmt.Sprintf("u%d_", i)
		b.WriteString(fmt.Sprintf(`CREATE type::record("user", $%sid) SET
			name = $%sname,
			email = $%semail,
			is_system_admin = $%sis_system_admin,
			oidc_provider = $%soidc_provider,
			oidc_subject = $%soidc_subject,
			created_at = $%screated_at;
		`, prefix, prefix, prefix, prefix, prefix, prefix, prefix))
		params[prefix+"id"] = bareID("user", userID)
		params[prefix+"name"] = u.Name
		params[prefix+"email"] = optionalString(u.Email)
		params[prefix+"is_system_admin"] = u.IsSystemAdmin
		params[prefix+"oidc_provider"] = optionalString(u.OIDCProvider)
		params[prefix+"oidc_subject"] = optionalString(u.OIDCSubject)
		params[prefix+"created_at"] = u.CreatedAt
	}

	for i, v := range snap.Vaults {
		vaultID, err := models.RecordIDString(v.ID)
		if err != nil {
			return fmt.Errorf("restore vault id: %w", err)
		}
		createdByID, err := models.RecordIDString(v.CreatedBy)
		if err != nil {
			return fmt.Errorf("restore vault created_by: %w", err)
		}

		var ownerVal any
		if v.Owner != nil {
			ownerID, err := models.RecordIDString(*v.Owner)
			if err != nil {
				return fmt.Errorf("restore vault owner: %w", err)
			}
			ownerVal = newRecordID("user", bareID("user", ownerID))
		} else {
			ownerVal = surrealmodels.None
		}

		var settingsVal any
		if v.Settings != nil {
			settingsVal = v.Settings
		} else {
			settingsVal = surrealmodels.None
		}

		prefix := fmt.Sprintf("v%d_", i)
		b.WriteString(fmt.Sprintf(`CREATE type::record("vault", $%sid) SET
			name = $%sname,
			description = $%sdescription,
			owner = $%sowner,
			settings = $%ssettings,
			created_by = type::record("user", $%screated_by),
			created_at = $%screated_at;
		`, prefix, prefix, prefix, prefix, prefix, prefix, prefix))
		params[prefix+"id"] = bareID("vault", vaultID)
		params[prefix+"name"] = v.Name
		params[prefix+"description"] = optionalString(v.Description)
		params[prefix+"owner"] = ownerVal
		params[prefix+"settings"] = settingsVal
		params[prefix+"created_by"] = bareID("user", createdByID)
		params[prefix+"created_at"] = v.CreatedAt
	}

	for i, m := range snap.VaultMembers {
		userID, err := models.RecordIDString(m.User)
		if err != nil {
			return fmt.Errorf("restore vault member user: %w", err)
		}
		vaultID, err := models.RecordIDString(m.Vault)
		if err != nil {
			return fmt.Errorf("restore vault member vault: %w", err)
		}

		prefix := fmt.Sprintf("m%d_", i)
		b.WriteString(fmt.Sprintf(`CREATE vault_member SET
			user = type::record("user", $%suser_id),
			vault = type::record("vault", $%svault_id),
			role = $%srole,
			created_at = $%screated_at;
		`, prefix, prefix, prefix, prefix))
		params[prefix+"user_id"] = bareID("user", userID)
		params[prefix+"vault_id"] = bareID("vault", vaultID)
		params[prefix+"role"] = m.Role
		params[prefix+"created_at"] = m.CreatedAt
	}

	for i, t := range snap.Tokens {
		userID, err := models.RecordIDString(t.User)
		if err != nil {
			return fmt.Errorf("restore token user: %w", err)
		}

		prefix := fmt.Sprintf("t%d_", i)
		b.WriteString(fmt.Sprintf(`CREATE api_token SET
			user = type::record("user", $%suser_id),
			token_hash = $%stoken_hash,
			name = $%sname,
			last_used = $%slast_used,
			expires_at = $%sexpires_at,
			created_at = $%screated_at;
		`, prefix, prefix, prefix, prefix, prefix, prefix))
		params[prefix+"user_id"] = bareID("user", userID)
		params[prefix+"token_hash"] = t.TokenHash
		params[prefix+"name"] = t.Name
		params[prefix+"last_used"] = optionalTime(t.LastUsed)
		params[prefix+"expires_at"] = optionalTime(t.ExpiresAt)
		params[prefix+"created_at"] = t.CreatedAt
	}

	b.WriteString("COMMIT TRANSACTION;")

	if _, err := surrealdb.Query[any](ctx, c.DB(), b.String(), params); err != nil {
		return fmt.Errorf("restore: %w", err)
	}

	logger.Info("restored identity",
		"users", len(snap.Users),
		"vaults", len(snap.Vaults),
		"vault_members", len(snap.VaultMembers),
		"tokens", len(snap.Tokens),
	)

	return nil
}
