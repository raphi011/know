# NFS File Access — Authentication Architecture

Technical reference for the NFS server's authentication model and future auth roadmap. For setup, mounting, and usage, see [feature-nfs.md](feature-nfs.md).

## Current Authentication

The NFS server uses NFSv3 null authentication — there is no per-user auth. All connected clients get system admin access to every vault. For security, the server always binds to `127.0.0.1` (localhost only).

> **Docker warning**: The `127.0.0.1` binding only protects bare-metal/VM setups. In Docker, `-p 2049:2049` forwards external traffic into the container, bypassing the localhost restriction. **Never map the NFS port externally** — anyone who can reach it gets unauthenticated admin access to all vaults. If you need remote file access, use WebDAV or SFTP which support token-based authentication.

## NFSv3 Auth Flavors

NFSv3 was designed for trusted LANs and has limited authentication options:

| Auth Flavor | How it works | Security | go-nfs support |
|-------------|-------------|----------|----------------|
| AUTH_NULL | No credentials | None | Yes (current) |
| AUTH_UNIX (AUTH_SYS) | Client self-reports UID/GID | Trivially spoofable — client picks any UID | Header is parsed, but not exposed to handler |
| AUTH_DES | DES + Diffie-Hellman key exchange | Obsolete, rarely implemented | No |
| RPCSEC_GSS (Kerberos) | Kerberos tickets, mutual auth, optional encryption | Strong | No |

None of these support bearer tokens like Know's other protocols (REST API, WebDAV, SSH/SFTP). The RPC credential field (`Auth{Flavor, Body}`) is an opaque byte slice — theoretically usable for custom auth, but no standard NFS client would populate it with a Know token.

## Authentication Roadmap

When remote NFS access is needed, these are the viable approaches (ranked by recommendation):

1. **HTTP pre-auth + mount secret**: Client POSTs their Know token to a REST endpoint, which registers a time-limited session and returns a mount secret. The NFS mount path includes this secret (`mount server:/session-abc123`). The server extracts it during the MOUNT RPC, validates the session, and scopes the filesystem to the token's vault permissions. The Know token never touches the NFS protocol.

2. **Token in mount path**: Embed the Know token directly in the NFS Dirpath (`mount server:/kh_abc123...`). The server extracts it in `Mount()`, calls `auth.Authenticate()`, and scopes vaults accordingly. Simpler than pre-auth but the token is visible in `mount` output and process listings.

3. **AUTH_UNIX UID mapping**: Configure UID-to-user mappings on the server. Standard NFS mount experience, but UIDs are trivially spoofable by any client on the network — only suitable for trusted LANs.

4. **Connection-scoped IP registration**: A sidecar mechanism registers a client IP via authenticated HTTP. The NFS server checks the remote IP on `Mount()` (available via `net.Conn`) and grants the associated permissions. Time-limited, but relies on IP-based trust.

## Implementation Notes

The plumbing for scoped auth already exists — the NFS filesystem is parameterized by `AuthContext`. Key extension points:

- **`Mount()` handler** (`internal/nfs/server.go`): Receives `MountRequest` (includes `Header.Cred` with auth credentials) and `net.Conn` (remote IP). This is where auth validation would happen.
- **`AuthContext` scoping** (`internal/nfs/fs.go`): The filesystem already filters vaults based on `AuthContext`. Replacing the hardcoded `IsSystemAdmin: true` with a token-derived context would immediately scope vault visibility.
- **`auth.Authenticate()`** (`internal/auth/validate.go`): The same function used by REST/WebDAV/SSH — returns an `AuthContext` with vault permissions. Any NFS auth scheme just needs to extract a token and call this.
