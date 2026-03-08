# SSH/SFTP File Access

## Context

Knowhow already has WebDAV for editor-based file access, but VS Code and Zed have better support for SSH remote connections than WebDAV. Adding an SFTP server over SSH lets these editors open the knowledge base directly as a remote workspace.

## Design

### Layout

SFTP root lists all accessible vaults as top-level directories:

```
/ (SFTP root)
├── default/
│   ├── docs/
│   │   └── guide.md
│   └── notes.md
└── another-vault/
    └── readme.md
```

### Auth

SSH password authentication where the password is a knowhow API token (username ignored), matching the WebDAV pattern. No public key auth for now.

### New package: `internal/sshd/`

Three files:

1. **`server.go`** — SSH server lifecycle
   - `Server` struct with `net.Listener`, services, ssh config
   - `NewServer(cfg)` — configures SSH server, loads/generates host key, sets up password callback
   - `Serve()` — accept loop, handle each connection in a goroutine
   - `Close()` — close listener, wait for active connections
   - Password callback: calls `auth.ValidateToken()`, stores user/vault info in `ssh.Permissions.Extensions`
   - Host key: load from `KNOWHOW_SSH_HOST_KEY` path, or auto-generate Ed25519 key to `~/.knowhow/host_key`

2. **`handler.go`** — SFTP request handlers implementing `sftp.Handlers` (FileReader, FileWriter, FileCmder, FileLister)
   - `Handler` struct with `db.Client`, `document.Service`, `vault.Service`, `auth.AuthContext`
   - `parsePath(p string) (vaultName, docPath string)` helper to split SFTP paths
   - Reuses same service calls as WebDAV (`docService.Create`, `db.GetDocumentByPath`, `vaultSvc.ListFolders`, etc.)
   - Only `.md` files allowed (same restriction as WebDAV)
   - Root `/` listing: `vaultSvc.List()` filtered by user's vault access
   - Vault-level operations delegate to document/vault services

3. **`handler_test.go`** — Unit tests for path parsing and basic handler logic

### Config changes: `internal/config/config.go`

Add three fields to `Config`:

```go
SSHEnabled     bool   // KNOWHOW_SSH_ENABLED (default: false)
SSHPort        string // KNOWHOW_SSH_PORT (default: "2222")
SSHHostKeyPath string // KNOWHOW_SSH_HOST_KEY (default: "" = auto-generate)
```

### Server integration: `cmd/knowhow-server/main.go`

After HTTP server setup, conditionally start SSH server:

```go
if cfg.SSHEnabled {
    sshSrv := sshd.NewServer(...)
    go sshSrv.Serve()
    defer sshSrv.Close()
}
```

### Dependencies

- `github.com/pkg/sftp` — SFTP subsystem handlers
- `golang.org/x/crypto/ssh` — SSH server (already indirect, promote to direct)

### Reuse from existing code

- `auth.ValidateToken()` — token validation in password callback
- `auth.AuthContext`, `auth.WithAuth()`, `auth.RequireVaultAccess()` — vault access checking
- `document.Service` methods: `Create`, `Delete`, `Move`
- `vault.Service` methods: `GetByName`, `List`, `ListFolders`, `CreateFolder`, `DeleteFolder`, `MoveFolder`
- `db.Client` methods: `GetDocumentByPath`, `GetDocumentMetaByPath`, `ListDocumentMetas`, `GetFolderByPath`
- Path normalization pattern from `internal/webdav/fs.go`
- `fileInfo` struct pattern from `internal/webdav/fs.go`
- No-auth mode pattern from `internal/webdav/handler.go`

### Design decisions

- **Opt-in** (`SSHEnabled=false` default) since it's a new network surface
- **No vault creation via SFTP** — mkdir at root returns permission denied
- **No WebDAV FS reuse** — the interfaces are incompatible (`webdav.FileSystem` vs `sftp.Handlers`) and the logic is simple enough that direct service calls are cleaner than wrapping
- **Username ignored** — only the token (password) matters, matching WebDAV

## Implementation steps

1. Add dependencies: `go get github.com/pkg/sftp@latest` (promotes `golang.org/x/crypto` to direct)
2. Add SSH config fields to `internal/config/config.go`
3. Create `internal/sshd/server.go` — SSH server, host key management, password auth
4. Create `internal/sshd/handler.go` — SFTP handlers (read/write/list/cmd)
5. Create `internal/sshd/handler_test.go` — unit tests for path parsing
6. Integrate SSH server startup/shutdown in `cmd/knowhow-server/main.go`
7. Update `README.md` with SFTP usage and editor configuration examples
8. `just build-all && just test`
