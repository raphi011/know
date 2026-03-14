# SSH/SFTP File Access

Access documents over SFTP for CLI and programmatic operations. The server exposes vaults as an SFTP filesystem, useful for scripted uploads, batch operations, and SFTP GUI clients (CyberDuck, Transmit, Filezilla).

**Note:** This is an SFTP-only server (no shell). Editor remote features (VS Code Remote SSH, Zed Remote Projects) require a full shell and won't work. For editor integration, use WebDAV instead.

## Overview

The SFTP server maps each accessible vault to a top-level directory. Documents appear as markdown files with the same folder structure as in the vault.

Key capabilities:

- **Vault-as-directory** — Each vault appears as a top-level folder under the SFTP root
- **Full CRUD** — Create, read, update, and delete documents and folders
- **Document pipeline** — Saves trigger parsing, embedding, linking, and chunking
- **Markdown-only** — Only `.md` files can be created
- **Token auth** — SSH password authentication using know API tokens

## Setup & Configuration

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `KNOW_SSH_ENABLED` | `false` | Enable the SSH/SFTP server |
| `KNOW_SSH_PORT` | `2222` | Port to listen on |
| `KNOW_SSH_HOST_KEY` | (auto-generate) | Path to host key file; generates Ed25519 key to `~/.know/host_key` if not set |

### Authentication

SSH password authentication where the password is a know API token (username is ignored), matching the WebDAV auth pattern.

## Usage

### Connecting

```bash
sftp -P 2222 anyuser@localhost
# Password: your API token
```

### Filesystem Layout

```
/ (SFTP root)
├── default/
│   ├── docs/
│   │   └── guide.md
│   └── notes.md
└── another-vault/
    └── readme.md
```

### Example Prompts

- "Upload all markdown files from `./notes/` into the `default` vault"
- "Download every document from the `research` vault for offline reading"
- "Batch rename files in a vault using a shell script over SFTP"
- "Sync a local folder with a know vault using `lftp mirror`"

### GUI Clients

Mount in CyberDuck, Transmit, or Filezilla using:

- **Protocol:** SFTP
- **Host:** `localhost`
- **Port:** `2222`
- **Password:** your API token

## Design Decisions

- **Opt-in** — Disabled by default since it adds a network surface
- **No vault creation via SFTP** — `mkdir` at root returns permission denied
- **Username ignored** — Only the token (password) matters, matching WebDAV
- **No WebDAV FS reuse** — The interfaces are incompatible (`webdav.FileSystem` vs `sftp.Handlers`) and direct service calls are cleaner than wrapping

## Reference

- Package: `internal/sshd/`
- Auth: SSH password (password = API token)
- Supported file type: Markdown (`.md`) only
- Dependencies: `github.com/pkg/sftp`, `golang.org/x/crypto/ssh`
