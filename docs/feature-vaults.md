# Vaults

Vaults are the top-level organizational unit in Knowhow, providing isolated document collections with role-based access control, folder hierarchies, and shareable public links.

## Overview

Every document belongs to exactly one vault. Vaults scope all operations -- search, sync, document management, and access control are all vault-relative. A user can be a member of multiple vaults with different roles, and each vault maintains its own folder tree, share links, and member list.

## Folders

Documents are organized into first-class folders backed by database records. Folders are not just path conventions -- they are persistent entities that support explicit CRUD operations.

**Key behaviors:**

- **Auto-creation**: When a document is added at a nested path, all ancestor folders are created automatically.
- **Explicit CRUD**: Folders can also be created, listed, and deleted directly.
- **Move**: `moveFolder` relocates a folder along with all its children (subfolders and documents) to a new path.
- **Cascading delete**: `deleteFolder` removes the folder and all nested content recursively.

### Listing files and folders

```bash
# List root of default vault
knowhow ls

# List a specific directory
knowhow ls /docs

# Recursive listing
knowhow ls /docs -R

# List in a specific vault
knowhow ls --vault my-vault /notes
```

## Access Control

Knowhow uses role-based access control at the vault level. Each vault member is assigned one of three roles:

| Role    | Level | Capabilities                                    |
|---------|-------|-------------------------------------------------|
| `read`  | 1     | Browse and read documents                       |
| `write` | 2     | Create, edit, delete documents                  |
| `admin` | 3     | Manage members, share links, vault settings     |

- Admins can add, update, and remove vault members.
- System admins bypass all role checks entirely.

## Share Links

Share links provide public, read-only access to documents or folders without requiring a user account.

**How it works:**

1. An admin creates a share link for a specific path, with an optional `expiresAt` timestamp.
2. A random token is generated. The raw token is returned only at creation time; the database stores a SHA256 hash.
3. The share token is used as a `Bearer` auth token, granting read-only access scoped to the vault and path.
4. If `isFolder=true`, access extends to all documents under that path prefix.

## Sync API

The sync endpoint supports metadata-first incremental sync for offline-capable clients (e.g. iOS app).

- **Without `since`**: Returns a full metadata dump (no tombstones).
- **With `since`**: Returns only documents updated after that timestamp, plus tombstones for deletions.
- Clients compare `contentHash` against their local cache to determine which documents need re-downloading.

## Usage

### Vault info

```bash
# Show stats for all vaults (documents, chunks, embeddings, labels, members, etc.)
knowhow vault

# Show stats for a specific vault
knowhow vault default

# Specify a custom API URL
knowhow vault my-vault --api-url http://localhost:4001
```

## Reference

- Vaults are defined in `internal/vault/` (CRUD, virtual folder derivation)
- Access control is enforced in `internal/auth/` (token validation, role checks)
- Folder operations live in `internal/document/` (create, move, delete with cascading)
- Share link logic is in `internal/auth/` (token generation, hash storage, scoped access)
- Sync endpoint is part of the REST API at `/api/sync`
- CLI commands: `knowhow vault`, `knowhow ls`
