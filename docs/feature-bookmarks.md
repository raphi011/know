# Bookmarks

Pin files and folders for quick access. Bookmarks are per-user and vault-scoped.

## Browse TUI

The `know browse` command includes a Bookmarks tab (press `3` to switch):

```bash
know browse --bookmarks     # start on the Bookmarks tab
know browse                 # start on All Files, press 3 to switch
```

While viewing any document, press `b` to toggle its bookmark status.

### Keyboard shortcuts

| Key | Context | Action |
|-----|---------|--------|
| `3` | Finding | Switch to Bookmarks tab |
| `b` | Viewing | Toggle bookmark on current document |
| `d` | Bookmarks tab | Remove selected bookmark |
| `enter` | Bookmarks tab | Open selected bookmark |
| `j`/`k` | Bookmarks tab | Navigate up/down |

## REST API

All endpoints require `Authorization: Bearer <token>`.

### List bookmarks

```
GET /api/v1/vaults/{vault}/bookmarks
```

Returns `FileEntry` objects for all bookmarked files in the vault.

### Add bookmark

```
PUT /api/v1/vaults/{vault}/bookmarks
Content-Type: application/json

{"path": "/notes/important.md"}
```

Idempotent — adding an existing bookmark is a no-op.

### Remove bookmark

```
DELETE /api/v1/vaults/{vault}/bookmarks
Content-Type: application/json

{"path": "/notes/important.md"}
```

Idempotent — removing a non-existent bookmark is a no-op.

## Storage

Bookmarks are stored in a `bookmark` table with a unique index on `(user, file)`. Cascade delete events on the `file` and `vault` tables ensure bookmarks are cleaned up automatically.
