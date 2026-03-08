package mcptools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/vault"
)

// resolveVaultIDs returns the list of vault IDs the caller has access to.
// In no-auth mode (wildcard access), it fetches all vault IDs from the DB.
func resolveVaultIDs(ctx context.Context, vaultService *vault.Service) ([]string, error) {
	ac, err := auth.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve vault IDs: %w", err)
	}

	// Check for wildcard access
	hasWildcard := false
	for _, vp := range ac.Vaults {
		if vp.VaultID == auth.WildcardVaultAccess {
			hasWildcard = true
			break
		}
	}

	if !hasWildcard {
		ids := make([]string, 0, len(ac.Vaults))
		for _, vp := range ac.Vaults {
			ids = append(ids, vp.VaultID)
		}
		return ids, nil
	}

	// Wildcard: resolve to all vaults
	vaults, err := vaultService.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list vaults: %w", err)
	}
	ids := make([]string, 0, len(vaults))
	for _, v := range vaults {
		id, err := models.RecordIDString(v.ID)
		if err != nil {
			slog.Warn("failed to extract vault ID, skipping", "vault_name", v.Name, "error", err)
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}
