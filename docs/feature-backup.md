# Backup & Restore

Manifest-based backup and restore for vaults. Creates portable `.tar.gz` archives containing all documents, assets, version history, and folder settings. Chunks and embeddings are omitted — they are regenerated on restore via the normal pipeline.

## Archive Format

```
know-backup-{vault}.tar.gz
├── manifest.json          # Vault metadata, file metadata, folder settings, version history
└── blobs/
    └── ab/cd/abcdef...    # Content-addressed blobs (same sharding as blob store)
```

The manifest contains:
- **Vault info**: name, description, settings
- **Files**: path, title, labels, doc_type, metadata, mime_type, content_hash, timestamps
- **Folders**: path, no_embed flag
- **Versions**: file_path, version number, content_hash, title, timestamp

Blobs are deduplicated — if a file and its version share the same content hash, the blob is written once.

## CLI Usage

### Backup

```bash
# Backup default vault
know backup --vault default

# Custom output path
know backup --vault default -o my-backup.tar.gz
```

### Restore

```bash
# Restore from archive (creates vault if it doesn't exist)
know restore know-backup-default.tar.gz
```

Restore:
1. Streams blobs directly into the blob store (no temp files)
2. Creates or reuses the vault by name
3. Applies vault settings
4. Creates folder structure with no_embed flags
5. Creates file records with content hashes
6. Enqueues pipeline jobs for text files (chunks/embeddings regenerated)
7. Restores version history

## REST API

### Export Backup

```
GET /api/backup?vault={vaultID}
Authorization: Bearer {token}
```

Returns: `application/gzip` stream with `Content-Disposition: attachment` header.

### Restore Backup

```
POST /api/backup/restore
Authorization: Bearer {token}
Content-Type: application/gzip
Body: <archive binary>
```

Request body limited to 1 GB. Returns JSON `{"status": "ok"}` on success.

## MCP / Agent Prompts

```
"Back up my default vault"
"Restore from know-backup-default.tar.gz"
"Create a backup of the work vault and save it to ~/backups/"
```

## Differences from Export

| Feature | Export (`/api/export`) | Backup (`/api/backup`) |
|---------|----------------------|----------------------|
| Format | tar.gz with raw files | tar.gz with manifest + blobs |
| Labels | Lost | Preserved |
| Doc types | Lost | Preserved |
| Metadata | Lost | Preserved |
| Folder settings | Lost | Preserved |
| Version history | Lost | Preserved |
| Vault settings | Lost | Preserved |
| Restoreable | No | Yes |
