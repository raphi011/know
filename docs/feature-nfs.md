# NFS File Access

Know includes an embedded NFSv3 server that exposes vaults as a network filesystem. This provides significantly faster file access than WebDAV, especially for bulk operations like copying many files.

## Why NFS over WebDAV?

WebDAV uses HTTP with XML-encoded requests — each file save requires 4+ round-trips (PROPFIND, LOCK, PUT, UNLOCK). NFS uses a persistent TCP connection with compact binary RPCs, dramatically reducing per-file overhead. For copying 100 files, WebDAV might do 400+ HTTP requests; NFS does ~200 RPCs on a single connection.

## Quick Start

### Enable the NFS Server

```bash
# Via environment variable
KNOW_NFS_ENABLED=true know serve

# Via CLI flag
know serve --nfs

# Custom port (default: 2049)
know serve --nfs --nfs-port 12049
```

The NFS server always binds to `127.0.0.1` (localhost only) for security — it uses NFSv3 null authentication which has no per-user access control.

### Mount on macOS

```bash
# Create mount point
mkdir -p /tmp/know-nfs

# Mount (specify port if non-standard)
mount_nfs -o tcp,port=2049,mountport=2049,vers=3 127.0.0.1:/ /tmp/know-nfs

# Or via Finder: Go → Connect to Server → nfs://127.0.0.1/
```

**Unmount:**
```bash
umount /tmp/know-nfs
```

### Mount on Linux

```bash
# Create mount point
sudo mkdir -p /mnt/know

# Mount
sudo mount -t nfs -o tcp,port=2049,mountport=2049,vers=3 127.0.0.1:/ /mnt/know

# Or add to /etc/fstab for auto-mount:
# 127.0.0.1:/ /mnt/know nfs tcp,port=2049,mountport=2049,vers=3 0 0
```

### Mount on Windows

Windows requires the "Services for NFS" optional feature:

1. Open **Settings** → **Apps** → **Optional features** → **Add a feature**
2. Install **Services for NFS**
3. Open Command Prompt as Administrator:

```cmd
mount -o port=2049 mtype=soft 127.0.0.1:/ Z:
```

## Directory Structure

Once mounted, vaults appear as top-level directories:

```
/tmp/know-nfs/
├── default/
│   ├── notes/
│   │   ├── meeting-notes.md
│   │   └── ideas.md
│   └── readme.md
├── work/
│   └── projects/
│       └── roadmap.md
└── personal/
    └── journal.md
```

## Supported Operations

| Operation | Supported | Notes |
|-----------|-----------|-------|
| Read files | Yes | Any file in any vault |
| Create files | Yes | Only `.md` files |
| Edit files | Yes | Only `.md` files |
| Delete files | Yes | Documents and folders |
| Rename/move | Yes | Within the same vault |
| Create folders | Yes | Via `mkdir` |
| Delete folders | Yes | Via `rmdir` |
| Cross-vault move | No | Rename across vaults not supported |
| Symlinks | No | Not applicable |
| File locking | No | NLM not implemented |

## Configuration

| Environment Variable | CLI Flag | Default | Description |
|---------------------|----------|---------|-------------|
| `KNOW_NFS_ENABLED` | `--nfs` | `false` | Enable the NFS server |
| `KNOW_NFS_PORT` | `--nfs-port` | `2049` | TCP port for NFS |

## Authentication

The NFS server uses NFSv3 null authentication — there is no per-user auth. For security:

- The server always binds to `127.0.0.1` (localhost only)
- All vaults accessible to the server are visible
- Network access from other machines is blocked

This matches the local development use case. For remote access, use WebDAV or SFTP which support token-based authentication.

## Limitations

- **Localhost only**: NFS server binds to 127.0.0.1, not accessible from other machines
- **No file locking**: NLM (Network Lock Manager) is not implemented; concurrent edits may conflict
- **Markdown only**: Only `.md` files can be created or edited; other file types are read-only
- **No portmap**: The server does not register with portmap/rpcbind; you must specify the port explicitly when mounting
- **Max file size**: 10 MB per document

## Troubleshooting

### "Operation not permitted" on macOS

macOS may require explicit port specification:
```bash
mount_nfs -o tcp,port=2049,mountport=2049,vers=3,nolocks 127.0.0.1:/ /tmp/know-nfs
```

### "Stale file handle"

The NFS server uses an in-memory handle cache. If the server restarts, mounted clients get stale handles. Unmount and remount:
```bash
umount /tmp/know-nfs
mount_nfs -o tcp,port=2049,mountport=2049,vers=3 127.0.0.1:/ /tmp/know-nfs
```

### Port already in use

If port 2049 is used by the system NFS server, use a different port:
```bash
know serve --nfs --nfs-port 12049
mount_nfs -o tcp,port=12049,mountport=12049,vers=3 127.0.0.1:/ /tmp/know-nfs
```

## Example Prompts

These prompts show what you can do with NFS-mounted vaults:

- **Bulk import**: `cp -r ~/notes/*.md /tmp/know-nfs/default/` — copy all markdown files into the default vault
- **Edit with any editor**: `vim /tmp/know-nfs/default/notes/ideas.md` — edit directly in your favorite editor
- **Search with grep**: `grep -r "TODO" /tmp/know-nfs/default/` — search across all documents
- **Organize files**: `mv /tmp/know-nfs/default/draft.md /tmp/know-nfs/default/published/` — move documents between folders
- **Backup a vault**: `cp -r /tmp/know-nfs/default/ ~/backup/` — copy all documents to a local backup
