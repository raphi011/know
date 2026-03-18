# Shell Completions for Vault File Operations

## Context

The CLI has no shell completion support. Users must type vault names and document paths manually. Adding completions improves usability for all path-based commands.

Additionally, the `mv` endpoint lacks validation for cross-type moves (folder to file path, file to folder path), which should be fixed alongside this work.

## Completion Command

Standard `know completion bash|zsh|fish|powershell` subcommand using cobra's built-in shell script generators. Outputs completion script to stdout for the user to source or install.

Usage:
```bash
# Fish
know completion fish > ~/.config/fish/completions/know.fish

# Zsh
know completion zsh > "${fpath[1]}/_know"

# Bash
know completion bash > /etc/bash_completion.d/know
```

## Flag Completions

### `--vault` flag

Registered via `RegisterFlagCompletionFunc` centrally in `addVaultFlag()`. Calls `GET /api/vaults`, returns vault names. Silent no-op if server is unreachable.

Applied to the 10 commands that use `addVaultFlag()`: ls, cat, rm, mv, labels, backup, agent, note, vault, vault settings.

The `import` command registers its own `--vault` flag (required, no default). Its vault completion is registered manually in `cmd_import.go`'s `init()` using the same `completeVaultNames` helper.

### `--api-url` / `--token`

No completion — freeform text. Use `cobra.ShellCompDirectiveNoFileComp` to suppress filesystem fallback.

## Positional Arg Completions

| Command | Arg | Completes | Filter |
|---------|-----|-----------|--------|
| `ls [path]` | 0 | vault paths | folders only |
| `cat <path>` | 0 | vault paths | files only |
| `rm <path>` | 0 | vault paths | files + folders |
| `mv <src> <dst>` | 0, 1 | vault paths | files + folders |
| `import <local> <vault-path>` | 0 | local filesystem | default cobra (file completion) |
| `import <local> <vault-path>` | 1 | vault paths | folders only |
| `vault [name]` | 0 | vault names | — |
| `vault settings [name]` | 0 | vault names | — |

Commands with no positional completion: labels, backup, agent, note, info, remote, db, serve, version.

### `import` special handling

The `import` command needs a custom `ValidArgsFunction` that branches on `len(args)`:
- `len(args) == 0` (completing arg 0): return `nil, cobra.ShellCompDirectiveDefault` to allow local filesystem completion for the source directory.
- `len(args) == 1` (completing arg 1): call `completeVaultPaths` with `pathFilterFolders` for the vault destination path.
- `len(args) >= 2`: return `nil, cobra.ShellCompDirectiveNoFileComp` (no more args).

If `--vault` is empty (not yet provided), vault path completion returns nothing silently.

## Implementation

### Shared completion helpers

New file `cmd/know/completions.go` with:

```go
// completeVaultNames returns vault names from the REST API.
func completeVaultNames(apiFlags *apiFlags) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
    // Calls GET /api/vaults, extracts names
    // Returns nil, cobra.ShellCompDirectiveNoFileComp on any error
}

type pathFilter int
const (
    pathFilterAll     pathFilter = iota // files + folders
    pathFilterFiles                     // files only
    pathFilterFolders                   // folders only
)

// completeVaultPaths returns vault paths from the REST API.
// Parses the typed prefix to determine the parent directory,
// calls GET /api/ls?vault={name}&path={parent}, filters results.
func completeVaultPaths(apiFlags *apiFlags, vaultFlag *string, filter pathFilter) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
    // ...
}
```

### Path prefix parsing

The `completeVaultPaths` helper parses `toComplete` to determine the parent directory for the API call:

- Empty string → list from `/`
- `/docs/` (trailing slash) → list contents of `/docs`
- `/docs/read` (no trailing slash) → list contents of `/docs`, filter by prefix `read`
- No leading `/` → prepend `/` (vault paths always start with `/`)

### Vault resolution during completion

If `*vaultFlag` is empty (user hasn't set `--vault` and there's no default), return nothing silently. This applies to the `import` command where `--vault` is required with no default.

### Wiring completions

Each command's `init()` registers its completion function:

- `addVaultFlag()` takes the `*apiFlags` parameter and calls `RegisterFlagCompletionFunc` for `--vault` using `completeVaultNames`
- `addAPIFlags()` calls `RegisterFlagCompletionFunc` for `--api-url` and `--token` with `cobra.ShellCompDirectiveNoFileComp`
- Each command sets `ValidArgsFunction` for positional args in its `init()`

### Completion directives

- `cobra.ShellCompDirectiveNoFileComp` — always set to prevent filesystem fallback (except `import` arg 0)
- `cobra.ShellCompDirectiveNoSpace` — set for path completions so users can keep typing subdirectories after `/`
- On error (server unreachable, auth failure) — return `nil, cobra.ShellCompDirectiveNoFileComp` (silent no-op)

### Constructing the API client for completions

The completion functions need an API client, but flag values may not be parsed yet during completion. Cobra parses flags before calling completion functions, so `apiFlags.URL` and `apiFlags.Token` are available. If both are empty, fall back to env vars (`KNOW_SERVER_URL`, `KNOW_TOKEN`).

## Move Endpoint Validation Fix

**File**: `internal/api/documents.go`, `move()` handler.

Add cross-type conflict checks:

1. After determining source is a **document** — check if destination path matches an existing folder. If so, return 409 Conflict with message "cannot move document to existing folder path".
2. After determining source is a **folder** — check if destination path matches an existing document. If so, return 409 Conflict with message "cannot move folder to existing document path".

The existing check (destination document conflicts with source document) is retained.

Folder-to-folder moves where the destination folder already has documents are allowed — this effectively merges the folder contents by rewriting path prefixes. This is the existing behavior and is intentional.

### Move validation tests

Add test cases in the existing handler test file covering:
- Document → existing folder path → 409
- Folder → existing document path → 409
- Document → document (existing check, already tested) → 409
- Folder → non-existent path → 200 (existing behavior)
- Folder → existing folder → 200 (merge, existing behavior)

## Files to Create/Modify

1. `cmd/know/completions.go` — new: shared completion helpers
2. `cmd/know/main.go` — add `completion` subcommand, update `addVaultFlag` signature, wire `addAPIFlags` completions
3. `cmd/know/cmd_ls.go` — add `ValidArgsFunction`
4. `cmd/know/cmd_cat.go` — add `ValidArgsFunction`
5. `cmd/know/cmd_rm.go` — add `ValidArgsFunction`
6. `cmd/know/cmd_mv.go` — add `ValidArgsFunction`
7. `cmd/know/cmd_import.go` — add `ValidArgsFunction`, register vault flag completion manually
8. `cmd/know/cmd_vault.go` — add `ValidArgsFunction`
9. `cmd/know/cmd_vault_settings.go` — add `ValidArgsFunction`
10. `internal/api/documents.go` — add cross-type move validation
11. `internal/api/documents_test.go` or `internal/integration/` — move validation tests

## Verification

1. `just build` — compiles
2. `just test` — all tests pass (including new move validation tests)
3. `know completion fish | source` — fish completions load without error
4. `know ls <TAB>` — completes folder paths from vault
5. `know browse <TAB>` — completes file paths from vault
6. `know rm <TAB>` — completes all paths from vault
7. `know mv <TAB>` — completes all paths for both args
8. `know --vault <TAB>` — completes vault names (on any command with the flag)
9. `know import --vault default ./local <TAB>` — completes vault folder paths
10. `know import ./local <TAB>` — no vault path completion (--vault not set), no error
11. With server stopped: `know ls <TAB>` — no error, no suggestions
12. Move validation: folder→document path returns 409, document→folder path returns 409
