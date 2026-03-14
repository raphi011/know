package mcptools

import (
	"context"
	"fmt"
	"strings"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/remote"
	"github.com/raphi011/know/internal/tools"
	"github.com/raphi011/know/internal/vault"
)

// VaultRef is an alias for tools.VaultRef used in MCP tool handlers.
type VaultRef = tools.VaultRef

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
			logutil.FromCtx(ctx).Warn("failed to extract vault ID, skipping", "vault_name", v.Name, "error", err)
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// resolveAllVaults returns VaultRefs for both local and remote vaults.
// Each ref carries the appropriate executor. Remote vaults are skipped
// if remoteService is nil or unreachable.
func (t *mcpTools) resolveAllVaults(ctx context.Context) ([]VaultRef, error) {
	localIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return nil, err
	}

	refs := make([]VaultRef, 0, len(localIDs))
	for _, id := range localIDs {
		refs = append(refs, VaultRef{
			VaultID:  id,
			Executor: t.executor,
		})
	}

	if t.remoteService == nil {
		return refs, nil
	}

	remoteVaults, err := t.remoteService.ListRemoteVaults(ctx)
	if err != nil {
		logutil.FromCtx(ctx).Warn("failed to list remote vaults, using local only", "error", err)
		return refs, nil
	}
	for _, rv := range remoteVaults {
		client, err := t.remoteService.ClientFor(ctx, rv.RemoteName)
		if err != nil {
			logutil.FromCtx(ctx).Warn("failed to get client for remote, skipping", "remote", rv.RemoteName, "error", err)
			continue
		}
		refs = append(refs, VaultRef{
			VaultID:   rv.VaultID,
			Executor:  remote.NewExecutor(client, rv.RemoteName),
			Namespace: rv.Namespace,
		})
	}

	return refs, nil
}

// resolveWriteVault resolves the target vault for a write operation.
// If vaultName contains "/" it routes to a remote vault.
// Otherwise, it uses the first local vault.
func (t *mcpTools) resolveWriteVault(ctx context.Context, vaultName string) (VaultRef, error) {
	if vaultName != "" && strings.Contains(vaultName, "/") {
		// Remote vault: "home/default" → remote="home", vaultName="default"
		parts := strings.SplitN(vaultName, "/", 2)
		remoteName := parts[0]

		if t.remoteService == nil {
			return VaultRef{}, fmt.Errorf("remote vaults not configured")
		}

		client, err := t.remoteService.ClientFor(ctx, remoteName)
		if err != nil {
			return VaultRef{}, fmt.Errorf("resolve remote %q: %w", remoteName, err)
		}

		// Find the vault ID on the remote by name
		remoteVaults, err := t.remoteService.ListRemoteVaults(ctx)
		if err != nil {
			return VaultRef{}, fmt.Errorf("list remote vaults: %w", err)
		}
		for _, rv := range remoteVaults {
			if rv.Namespace == vaultName {
				return VaultRef{
					VaultID:   rv.VaultID,
					Executor:  remote.NewExecutor(client, remoteName),
					Namespace: rv.Namespace,
				}, nil
			}
		}
		return VaultRef{}, fmt.Errorf("remote vault %q not found", vaultName)
	}

	// Local vault
	vaultIDs, err := resolveVaultIDs(ctx, t.vaultService)
	if err != nil {
		return VaultRef{}, err
	}
	if len(vaultIDs) == 0 {
		return VaultRef{}, fmt.Errorf("no vaults accessible")
	}

	return VaultRef{
		VaultID:  vaultIDs[0],
		Executor: t.executor,
	}, nil
}
