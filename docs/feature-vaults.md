# Vaults

Vaults are the top-level organizational unit in Know, providing isolated document collections with role-based access control, folder hierarchies, and shareable public links.

## Overview

Every document belongs to exactly one vault. Vaults scope all operations -- search, sync, document management, and access control are all vault-relative. A user can be a member of multiple vaults with different roles, and each vault maintains its own folder tree, share links, and member list.

## Folders

Documents are organized into first-class folders backed by database records. Folders are not just path conventions -- they are persistent entities that support explicit CRUD operations.

**Key behaviors:**

- **Auto-creation**: When a document is added at a nested path, all ancestor folders are created automatically.
- **Explicit CRUD**: Folders can also be created, listed, and deleted directly.
- **Move**: `moveFolder` relocates a folder along with all its children (subfolders and documents) to a new path.
- **Cascading delete**: `deleteFolder` removes the folder and all nested content recursively.

### Disabling embeddings per folder

Folders support a `noEmbed` flag that prevents embedding generation for all documents underneath them. When enabled, existing embeddings are stripped and future files skip the embed step during ingestion.

**Use cases:**
- Large archive folders where semantic search isn't needed
- Raw data folders (CSV dumps, logs) that would produce low-quality embeddings
- Reducing embedding costs by excluding irrelevant content

**API usage:**

```bash
# Disable embeddings for /archive and strip existing ones
curl -X PATCH http://localhost:4000/api/folders \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"vault": "default", "path": "/archive", "noEmbed": true}'
# Response: {"strippedChunks": 42}

# Re-enable embeddings (new documents will be embedded on next import)
curl -X PATCH http://localhost:4000/api/folders \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"vault": "default", "path": "/archive", "noEmbed": false}'

# Check which folders have noEmbed set
curl "http://localhost:4000/api/folders?vault=default" \
  -H "Authorization: Bearer $TOKEN"
```

**Behavior details:**
- Setting `noEmbed: true` immediately strips embeddings from all chunks under the folder.
- The flag is inherited by all descendant paths — a file at `/archive/2024/report.md` is affected by `noEmbed` on `/archive`.
- Setting `noEmbed: false` does not retroactively re-embed — re-import the documents to generate new embeddings.
- The check fails open: if the DB is temporarily unavailable, files are embedded rather than silently skipped.

### Listing files and folders

```bash
# List root of default vault
know ls

# List a specific directory
know ls /docs

# Recursive listing
know ls /docs -R

# List in a specific vault
know ls --vault my-vault /notes
```

## Access Control

Know uses role-based access control at the vault level. Each vault member is assigned one of three roles:

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

## Usage

### Vault info

```bash
# Show stats for all vaults (documents, chunks, embeddings, labels, members, etc.)
know vault

# Show stats for a specific vault
know vault default

# Specify a custom API URL
know vault my-vault --api-url http://localhost:4001
```

## Reference

- Vaults are defined in `internal/vault/` (CRUD, virtual folder derivation)
- Access control is enforced in `internal/auth/` (token validation, role checks)
- Folder operations live in `internal/document/` (create, move, delete with cascading)
- Share link logic is in `internal/auth/` (token generation, hash storage, scoped access)
- CLI commands: `know vault`, `know ls`
