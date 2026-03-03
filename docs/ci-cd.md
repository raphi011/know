# CI/CD — GitHub Actions Deploy Pipeline

## Overview

The deploy pipeline (`.github/workflows/deploy.yml`) builds ARM64 Docker images and pushes them along with the Helm chart to the private [Zot](https://zot.manx-turtle.ts.net) OCI registry on the Turing Pi cluster. It connects to the cluster via **Tailscale** using an OAuth client.

ArgoCD then picks up the new images/chart from Zot and reconciles the deployment.

```
GitHub Actions (ARM64 runner)
  │
  ├── Tailscale ──► zot.manx-turtle.ts.net (OCI registry)
  │
  ├── docker build + push  (backend image)
  ├── docker build + push  (frontend image)
  └── helm package + push  (Helm chart as OCI artifact)
         │
         ▼
    ArgoCD (polls Zot) ──► deploys to knowhow namespace
```

## Triggers

| Trigger | Condition |
|---------|-----------|
| Push to `main` | Only when matching path filters (see below) |
| `workflow_dispatch` | Manual — runs all jobs regardless of path filters |

## Jobs & Path Filters

The pipeline uses `dorny/paths-filter` to detect which components changed and only builds what's needed:

| Job | Runs when changed | Runner | Builds |
|-----|-------------------|--------|--------|
| `build-backend` | `**.go`, `go.mod`, `go.sum`, `Dockerfile` | `ubuntu-24.04-arm` | `knowhow` image |
| `build-frontend` | `web/**` | `ubuntu-24.04-arm` | `knowhow-web` image |
| `push-helm` | `helm/**` | `ubuntu-latest` | Helm OCI artifact |

All three jobs run on `workflow_dispatch` regardless of path filters.

## Versioning

Image tags and chart version are derived from `appVersion` in `helm/knowhow/Chart.yaml`. Both `:version` and `:latest` tags are pushed for images.

To release a new version:
1. Bump `version` and `appVersion` in `Chart.yaml`
2. Update `targetRevision` in the ArgoCD Application (`turingpi-k8s/argocd/apps/knowhow/application.yaml`)
3. Push to `main`

## Tailscale Network Access

Each job connects to the Tailscale network to reach the Zot registry. This requires:

### GitHub Secrets

| Secret | Description |
|--------|-------------|
| `TS_OAUTH_CLIENT_ID` | Tailscale OAuth client ID |
| `TS_OAUTH_SECRET` | Tailscale OAuth client secret |

### OAuth Client Configuration

Created at [Tailscale Admin → Settings → OAuth Clients](https://login.tailscale.com/admin/settings/oauth):

- **Tag**: `tag:ci` (nodes authenticate as this tag)
- **Scopes**: Devices → Core → Write, Keys → Auth Keys → Write

### Tailscale ACL Requirements

The `tag:ci` tag needs ACL rules allowing:

```jsonc
// In Tailscale ACLs
{
  "action": "accept",
  "src": ["tag:ci"],
  "dst": ["tag:k8s:5000", "autogroup:internet:*"]
}
```

- `tag:k8s:5000` — access to Zot registry (port 5000) on cluster nodes
- `autogroup:internet:*` — internet access for pulling base images during `docker build`

### DNS Resolution (the key trick)

Tailscale's MagicDNS takes over `systemd-resolved` on Linux runners, which breaks public DNS resolution (Docker Hub, GitHub, etc.). We need Tailscale hostnames to resolve while keeping public DNS working.

```yaml
- name: Connect to Tailscale
  uses: tailscale/github-action@v3
  with:
    oauth-client-id: ${{ secrets.TS_OAUTH_CLIENT_ID }}
    oauth-secret: ${{ secrets.TS_OAUTH_SECRET }}
    tags: tag:ci
    args: --accept-dns=false  # Don't let Tailscale take over DNS

- name: Add Zot registry to /etc/hosts
  run: |
    ZOT_IP=$(tailscale ip -4 zot)
    echo "$ZOT_IP zot.manx-turtle.ts.net" | sudo tee -a /etc/hosts
```

**How it works:**
1. `--accept-dns=false` prevents Tailscale from overriding the system DNS resolver — public DNS stays intact
2. `tailscale ip -4 zot` resolves the Tailscale peer "zot" to its IPv4 Tailscale IP (e.g., `100.85.50.48`)
3. Adding to `/etc/hosts` ensures Docker/Helm can resolve the registry hostname without relying on `systemd-resolved` routing domains

**Why not `resolvectl` split DNS?** While `resolvectl domain tailscale0 '~manx-turtle.ts.net'` should theoretically work, it's unreliable across GitHub runner types (particularly ARM64 runners). The `/etc/hosts` approach is simpler and works everywhere.

See [tailscale/github-action#101](https://github.com/tailscale/github-action/issues/101) for background on the MagicDNS issue.

## ARM64 Native Builds

Backend and frontend jobs use `ubuntu-24.04-arm` runners for native ARM64 builds — no QEMU emulation needed. The Helm push job uses `ubuntu-latest` (x86) since it doesn't build images.

## Adding a New Component

To add a new build job:

1. Add path filter in the `changes` job
2. Add a new job following the existing pattern (Tailscale → split DNS → version → build → push)
3. Add the `if` condition referencing the filter output
