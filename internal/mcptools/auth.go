package mcptools

import (
	"context"
	"fmt"
	"slices"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/vault"
)

// resolveVaultIDs returns the list of vault IDs the caller has access to.
// In no-auth mode (wildcard access), it fetches all vault IDs from the DB.
func resolveVaultIDs(ctx context.Context, vaultService *vault.Service) ([]string, error) {
	ac, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	if !slices.Contains(ac.VaultAccess, auth.WildcardVaultAccess) {
		return ac.VaultAccess, nil
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
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}
