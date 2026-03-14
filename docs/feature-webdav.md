# WebDAV Server

Edit documents with any WebDAV-compatible editor. The server exposes vaults as WebDAV filesystems at `/dav/{vaultName}/`, with full document pipeline integration on every save.

## Overview

The WebDAV server maps each vault to a WebDAV endpoint where documents appear as markdown files and folders mirror the vault's folder structure. Writing a file triggers the full document pipeline — parse, embed, link, chunk — so edits made through any WebDAV client are immediately indexed and searchable.

Key capabilities:

- **Full CRUD** — Create, read, update, and delete documents and folders
- **Document pipeline** — Every save triggers parsing, embedding, linking, and chunking
- **Markdown-only** — Only `.md` files can be created or renamed
- **Optimistic concurrency** — Content-hash ETags for conflict detection
- **Per-vault locking** — Each vault gets its own WebDAV lock system
- **Role-based access** — Write methods require `RoleWrite`; read-only users can browse and read
- **macOS compatibility** — Silently handles `._*` resource forks and `.DS_Store`
- **Safety** — Rejects `PROPFIND Depth:infinity` (DoS prevention) and enforces max PUT body size

## Setup & Configuration

### Authentication

Auth uses HTTP Basic Auth. The username can be anything; the password is a know API token. A `noAuth` mode is available for local development.

### Mounting a Vault

**macOS Finder:**

Go > Connect to Server, then enter:

```
http://localhost:8484/dav/default/
```

When prompted, enter any username and your API token as the password.

**cadaver CLI:**

```bash
cadaver http://localhost:8484/dav/default/
# Username: anything
# Password: your API token
```

## Usage

### Editor Support

Any WebDAV client works: macOS Finder, Transmit, Cyberduck, or VS Code with a WebDAV extension. Mount `/dav/default/` and edit markdown files directly — changes sync back through the full document pipeline.

### Workflow

1. Mount the vault using your preferred WebDAV client
2. Browse the folder structure and open any `.md` file
3. Edit and save — the document pipeline processes the change automatically
4. New documents must use the `.md` extension

## Reference

- Endpoint pattern: `/dav/{vaultName}/`
- Auth: HTTP Basic Auth (password = API token)
- Supported file type: Markdown (`.md`) only
- Concurrency control: Content-hash ETags
- Lock scope: Per-vault
