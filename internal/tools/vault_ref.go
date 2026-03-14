package tools

import "context"

// VaultRef is a resolved vault reference with its executor and namespace.
type VaultRef struct {
	VaultID   string       // vault ID (local bare ID or remote server vault ID)
	Executor  ToolExecutor // local or remote executor
	Namespace string       // "" for local, "namespace/vault" for remote
}

// IsRemote returns true if this vault reference points to a remote vault.
func (r VaultRef) IsRemote() bool { return r.Namespace != "" }

// VaultResolver returns all accessible vaults (local + remote) for the current auth context.
type VaultResolver func(ctx context.Context) ([]VaultRef, error)

// WriteVaultResolver resolves a single vault for write operations.
// If vaultName contains "/" it routes to a remote vault; otherwise uses the first local vault.
type WriteVaultResolver func(ctx context.Context, vaultName string) (VaultRef, error)
