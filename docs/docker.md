# Docker / Compose / Colima

Technical learnings about running the local Docker Compose stack.

## Colima Setup

Colima is a lightweight Docker runtime for macOS (alternative to Docker Desktop).

### Memory Requirements

The default 2 GB is insufficient — Next.js Turbopack builds get OOM-killed (`SIGKILL` during `bun run build`). Allocate at least 4 GB:

```bash
colima stop
colima start --memory 4
```

Check current allocation:

```bash
colima list
# PROFILE    STATUS     ARCH       CPUS    MEMORY    DISK
# default    Running    aarch64    2       4GiB      100GiB
```

### Docker Compose Plugin

Colima may not include the `docker compose` v2 plugin. If `docker compose` fails with `unknown shorthand flag: 'f'`, install it:

```bash
brew install docker-compose
```

The justfile uses v2 syntax (`docker compose`, no hyphen). The standalone `docker-compose` (hyphen, v1) works as a fallback for manual commands.

## SurrealDB v3 Docker Image

### Storage Backend

The `file:` prefix is **deprecated in v3**. Use `surrealkv://` or `rocksdb://`:

```yaml
# BAD: deprecated, fails with "Unable to load the specified datastore"
command: start --user root --pass root file:/data/knowhow.db

# GOOD: SurrealKV (SurrealDB's native KV store)
command: start --user root --pass root surrealkv:/data/knowhow.db
```

### Healthcheck

The v3 image is scratch-based — no `ls`, `sh`, `which`, or other shell utilities. Only `/surreal` exists.

Key v3 CLI changes:
- `isready` → `is-ready` (hyphenated)
- `--conn` → `--endpoint`
- Default endpoint is `ws://localhost:8000` (no flag needed)

```yaml
# BAD: binary not on PATH, old command name, old flag
test: ["CMD", "surreal", "isready", "--conn", "http://localhost:8000"]

# GOOD: absolute path, v3 command, default endpoint
test: ["CMD", "/surreal", "is-ready"]
```

Must use `CMD` (exec form), not `CMD-SHELL` — there's no shell in the image.

## Troubleshooting

### SurrealDB "Unable to load the specified datastore"

1. Check storage prefix — use `surrealkv://` not `file:`
2. Wipe stale data: `rm -rf data/surreal`
3. If persists, check volume permissions under Colima's virtiofs

### Next.js Build Killed (SIGKILL)

OOM during `bun run build` in Docker. Increase Colima memory to 4+ GB (see above).

### Healthcheck Stuck "unhealthy"

SurrealDB appears running in logs but stays unhealthy → the healthcheck command itself is failing. Verify manually:

```bash
docker compose exec surrealdb /surreal is-ready
```
