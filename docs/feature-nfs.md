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
# Mount to /Volumes/know (Finder-friendly)
sudo mkdir -p /Volumes/know
sudo mount_nfs -o tcp,port=2049,mountport=2049,vers=3,nolocks 127.0.0.1:/ /Volumes/know
```

> **Note:** `nolocks` is required because the NFS server does not implement NLM (Network Lock Manager). Use `/Volumes/know` instead of `/tmp/` so Finder can browse the mount correctly (macOS's `/tmp` → `/private/tmp` symlink confuses Finder).

**Unmount:**
```bash
sudo umount /Volumes/know
```

### Mount on Linux

```bash
# Create mount point
sudo mkdir -p /mnt/know

# Mount
sudo mount -t nfs -o tcp,port=2049,mountport=2049,vers=3,nolock 127.0.0.1:/ /mnt/know

# Or add to /etc/fstab for auto-mount:
# 127.0.0.1:/ /mnt/know nfs tcp,port=2049,mountport=2049,vers=3,nolock 0 0
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
/Volumes/know/          # macOS — or /mnt/know/ on Linux
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

The NFS server uses NFSv3 null authentication — there is no per-user auth. All connected clients get system admin access to every vault. For security, the server always binds to `127.0.0.1` (localhost only).

> **Docker warning**: The `127.0.0.1` binding only protects bare-metal/VM setups. In Docker, `-p 2049:2049` forwards external traffic into the container, bypassing the localhost restriction. **Never map the NFS port externally** — anyone who can reach it gets unauthenticated admin access to all vaults. If you need remote file access, use WebDAV or SFTP which support token-based authentication.

### Why no per-user auth?

NFSv3 was designed for trusted LANs and has limited authentication options:

| Auth Flavor | How it works | Security | go-nfs support |
|-------------|-------------|----------|----------------|
| AUTH_NULL | No credentials | None | Yes (current) |
| AUTH_UNIX (AUTH_SYS) | Client self-reports UID/GID | Trivially spoofable — client picks any UID | Header is parsed, but not exposed to handler |
| AUTH_DES | DES + Diffie-Hellman key exchange | Obsolete, rarely implemented | No |
| RPCSEC_GSS (Kerberos) | Kerberos tickets, mutual auth, optional encryption | Strong | No |

None of these support bearer tokens like Know's other protocols (REST API, WebDAV, SSH/SFTP). The RPC credential field (`Auth{Flavor, Body}`) is an opaque byte slice — theoretically usable for custom auth, but no standard NFS client would populate it with a Know token.

### Authentication roadmap

When remote NFS access is needed, these are the viable approaches (ranked by recommendation):

1. **HTTP pre-auth + mount secret**: Client POSTs their Know token to a REST endpoint, which registers a time-limited session and returns a mount secret. The NFS mount path includes this secret (`mount server:/session-abc123`). The server extracts it during the MOUNT RPC, validates the session, and scopes the filesystem to the token's vault permissions. The Know token never touches the NFS protocol.

2. **Token in mount path**: Embed the Know token directly in the NFS Dirpath (`mount server:/kh_abc123...`). The server extracts it in `Mount()`, calls `auth.Authenticate()`, and scopes vaults accordingly. Simpler than pre-auth but the token is visible in `mount` output and process listings.

3. **AUTH_UNIX UID mapping**: Configure UID-to-user mappings on the server. Standard NFS mount experience, but UIDs are trivially spoofable by any client on the network — only suitable for trusted LANs.

4. **Connection-scoped IP registration**: A sidecar mechanism registers a client IP via authenticated HTTP. The NFS server checks the remote IP on `Mount()` (available via `net.Conn`) and grants the associated permissions. Time-limited, but relies on IP-based trust.

### Implementation notes for future work

The plumbing for scoped auth already exists — the NFS filesystem is parameterized by `AuthContext`. Key extension points:

- **`Mount()` handler** (`internal/nfs/server.go`): Receives `MountRequest` (includes `Header.Cred` with auth credentials) and `net.Conn` (remote IP). This is where auth validation would happen.
- **`AuthContext` scoping** (`internal/nfs/fs.go`): The filesystem already filters vaults based on `AuthContext`. Replacing the hardcoded `IsSystemAdmin: true` with a token-derived context would immediately scope vault visibility.
- **`auth.Authenticate()`** (`internal/auth/validate.go`): The same function used by REST/WebDAV/SSH — returns an `AuthContext` with vault permissions. Any NFS auth scheme just needs to extract a token and call this.

## Limitations

- **Localhost only**: NFS server binds to 127.0.0.1, not accessible from other machines (but see Docker warning in Authentication section)
- **No file locking**: NLM (Network Lock Manager) is not implemented; concurrent edits may conflict
- **Markdown only**: Only `.md` files can be created or edited; other file types are read-only
- **No portmap**: The server does not register with portmap/rpcbind; you must specify the port explicitly when mounting
- **Max file size**: 10 MB per document

## Troubleshooting

### "Operation not permitted" on macOS

macOS may require explicit port specification and `sudo`:
```bash
sudo mount_nfs -o tcp,port=2049,mountport=2049,vers=3,nolocks 127.0.0.1:/ /Volumes/know
```

### "Stale file handle"

The NFS server uses an in-memory handle cache. If the server restarts, mounted clients get stale handles. Unmount and remount:
```bash
sudo umount /Volumes/know
sudo mount_nfs -o tcp,port=2049,mountport=2049,vers=3,nolocks 127.0.0.1:/ /Volumes/know
```

### Port already in use

If port 2049 is used by the system NFS server, use a different port:
```bash
know serve --nfs --nfs-port 12049
sudo mount_nfs -o tcp,port=12049,mountport=12049,vers=3,nolocks 127.0.0.1:/ /Volumes/know
```

## Example Prompts

These prompts show what you can do with NFS-mounted vaults:

- **Bulk import**: `cp -r ~/notes/*.md /Volumes/know/default/` — copy all markdown files into the default vault
- **Edit with any editor**: `vim /Volumes/know/default/notes/ideas.md` — edit directly in your favorite editor
- **Search with grep**: `grep -r "TODO" /Volumes/know/default/` — search across all documents
- **Organize files**: `mv /Volumes/know/default/draft.md /Volumes/know/default/published/` — move documents between folders
- **Backup a vault**: `cp -r /Volumes/know/default/ ~/backup/` — copy all documents to a local backup
