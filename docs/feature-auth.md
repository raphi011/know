# Authentication

Know supports two authentication methods: API tokens (bearer tokens) and OIDC (OpenID Connect) for browser-based login. Both methods produce `kh_`-prefixed API tokens that are used for all subsequent API calls.

## Overview

- **Token auth** is the default and simplest method. Create tokens via `know db seed` or the Token Management API. Tokens are SHA256-hashed before storage -- the raw token is shown exactly once at creation.
- **OIDC auth** adds browser-based login via any OIDC provider (GitHub, Google, Okta, etc.). Two flows are supported: device flow for CLIs and PKCE flow for native apps.

## Token Authentication

All API requests require a bearer token in the `Authorization` header:

```bash
curl -H "Authorization: Bearer kh_abc123..." http://localhost:4001/api/v1/vaults
```

Tokens are prefixed with `kh_` for easy identification. They are scoped to a user and inherit that user's vault memberships and roles.

### Token lifecycle

- **Creation**: via `know db seed`, the Token Management API, or OIDC login flows
- **Expiry**: tokens have an expiry date, governed by `KNOW_TOKEN_MAX_LIFETIME_DAYS` (default: 90 days)
- **Rotation**: atomically replace a token while preserving its remaining TTL
- **Revocation**: delete a token to immediately revoke access

### Environment variable

Set `KNOW_TOKEN` to avoid passing `--token` on every CLI command:

```bash
export KNOW_TOKEN=kh_abc123...
know vault
```

## Token Management API

Authenticated users can manage their own tokens via the REST API.

### List tokens

```bash
# GET /api/v1/tokens
curl http://localhost:4001/api/v1/tokens \
  -H "Authorization: Bearer $TOKEN"
```

Response:
```json
[
  {
    "id": "abc123",
    "name": "my-laptop",
    "last_used": "2026-03-19T10:00:00Z",
    "expires_at": "2026-06-17T10:00:00Z",
    "created_at": "2026-03-19T10:00:00Z"
  }
]
```

### Create token

```bash
# POST /api/v1/tokens
curl -X POST http://localhost:4001/api/v1/tokens \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name": "ci-pipeline", "expires_in_days": 30}'
```

Response includes the raw token (shown once):
```json
{
  "raw_token": "kh_newtoken...",
  "token": {
    "id": "def456",
    "name": "ci-pipeline",
    "expires_at": "2026-04-18T10:00:00Z",
    "created_at": "2026-03-19T10:00:00Z"
  }
}
```

If `expires_in_days` is omitted, the server's `KNOW_TOKEN_MAX_LIFETIME_DAYS` is used. The request is rejected if `expires_in_days` exceeds the server maximum.

### Delete token

```bash
# DELETE /api/v1/tokens/{id}
curl -X DELETE http://localhost:4001/api/v1/tokens/abc123 \
  -H "Authorization: Bearer $TOKEN"
# 204 No Content
```

Users can only delete their own tokens. System admins can delete any token.

### Rotate token

```bash
# POST /api/v1/tokens/{id}/rotate
curl -X POST http://localhost:4001/api/v1/tokens/abc123/rotate \
  -H "Authorization: Bearer $TOKEN"
```

Creates a new token with the same name and remaining TTL, then deletes the old one. Returns the new raw token in the response.

## OIDC Setup

OIDC enables browser-based login via external identity providers. When enabled, Know exposes auth endpoints for device flow (CLI) and PKCE flow (native apps).

Two provider types are supported:

- **`oidc`** (default): Standard OIDC providers with discovery (Google, Okta, Auth0, Keycloak, etc.)
- **`github`**: GitHub OAuth Apps (GitHub does not support OIDC discovery, so a dedicated provider fetches user info via the GitHub REST API)

### GitHub OAuth App

1. Go to GitHub Settings > Developer Settings > OAuth Apps > New OAuth App
2. Set:
   - **Application name**: Know
   - **Homepage URL**: `https://know.example.com`
   - **Authorization callback URL**: `https://know.example.com/auth/callback`
3. Note the Client ID and generate a Client Secret

4. Configure the server:

```bash
export KNOW_OIDC_ENABLED=true
export KNOW_OIDC_PROVIDER_TYPE=github
export KNOW_OIDC_CLIENT_ID=your-client-id
export KNOW_OIDC_CLIENT_SECRET=your-client-secret
export KNOW_OIDC_REDIRECT_URL=https://know.example.com/auth/callback
```

Or in the Helm chart:

```yaml
oidc:
  enabled: true
  providerType: "github"
  clientID: "your-client-id"
  clientSecret: "your-client-secret"
  redirectURL: "https://know.example.com/auth/callback"
  selfSignupEnabled: false
```

Note: `issuerURL` is not needed for GitHub -- the provider uses GitHub's well-known OAuth2 and API endpoints directly. The user's stable numeric GitHub ID is used as the identity subject (not the login, which can change).

**PKCE limitation**: GitHub OAuth Apps do not support PKCE (`code_verifier` is silently ignored). The device flow (CLI) is unaffected, but native apps using the PKCE flow (`POST /auth/token`) will work without actual PKCE protection. If PKCE is required, consider using a GitHub App instead of an OAuth App.

### Standard OIDC Provider (Google, Okta, etc.)

For providers with OIDC discovery:

```bash
export KNOW_OIDC_ENABLED=true
export KNOW_OIDC_PROVIDER_TYPE=oidc  # default, can be omitted
export KNOW_OIDC_ISSUER_URL=https://accounts.google.com
export KNOW_OIDC_CLIENT_ID=your-client-id
export KNOW_OIDC_CLIENT_SECRET=your-client-secret
export KNOW_OIDC_REDIRECT_URL=https://know.example.com/auth/callback
```

### Auth endpoints

When OIDC is enabled, the following unauthenticated endpoints are registered:

| Method | Path                  | Purpose                                |
|--------|-----------------------|----------------------------------------|
| POST   | `/auth/device/start`  | Start device flow (returns user code)  |
| POST   | `/auth/device/poll`   | Poll device flow for token             |
| GET    | `/auth/login`         | Redirect to OIDC provider              |
| GET    | `/auth/callback`      | Handle OIDC provider redirect          |
| POST   | `/auth/token`         | Exchange code + PKCE verifier for token|

## CLI Login

The `know auth` command group manages authentication from the terminal.

### `know auth login`

The CLI uses a **try-and-fallback** approach to discover available auth methods:
it attempts the device flow (`POST /auth/device/start`) first. If it succeeds,
OIDC is available and the user is offered a choice. If it fails (404 when OIDC
is disabled, or any other error), the CLI falls back to token paste.

1. **Browser login (OIDC)** -- uses the device flow:
   - Server generates a user code (e.g. `ABCD-EFGH`) and device code
   - CLI displays the user code and opens the browser to the verification URL
   - User authenticates with the OIDC provider in the browser
   - CLI polls until the device code is approved, then saves the token

2. **Token paste** -- manually enter an existing API token:
   - CLI validates the token format (`kh_` prefix)
   - Verifies the token works by calling the API
   - Saves to the system keychain (macOS Keychain, Linux libsecret, Windows Credential Manager)

```bash
# Login to default server
know auth login

# Login to a specific server
know auth login --api-url https://know.example.com
```

### `know auth status`

Shows current authentication state:

```bash
know auth status
# Token source: system keychain
# Server: https://know.example.com
# Token: kh_abc...xyz9
```

### `know auth logout`

Removes the stored token:

```bash
know auth logout
# Logged out successfully.
```

### Token resolution order

The CLI resolves tokens in this order:
1. `KNOW_TOKEN` environment variable (recommended for CI/headless systems)
2. System keychain (saved by `know auth login`)
3. `--token` flag (if supported by the command)

### OAuth MCP Authentication

When OIDC is enabled, Know exposes an OAuth 2.0 Authorization Server facade on the protocol port. This lets Claude Code and other MCP clients authenticate via browser login without manual token configuration.

**Endpoints** (on protocol port, default 4002):
- `GET /.well-known/oauth-authorization-server` — OAuth metadata (RFC 8414)
- `GET /oauth/authorize` — Starts the auth flow (redirects to OIDC provider)
- `POST /oauth/token` — Exchanges authorization code for `kh_` token

**Setup:**
```bash
claude mcp add --transport http --client-id know-mcp know http://localhost:4002/mcp
```

Then in Claude Code, run `/mcp` and select "Authenticate" — a browser window opens, you log in with your OIDC provider, and the token is issued and stored automatically by the MCP client.

**Configuration:**
- `KNOW_PROTOCOL_BASE_URL` — Public URL of the protocol port (required in production, e.g. `https://know.example.com:4002`)
- Requires `KNOW_OIDC_ENABLED=true` and a configured OIDC provider

## Native App Login (PKCE)

Native apps (iOS, macOS) use the PKCE (Proof Key for Code Exchange) flow:

1. App generates a PKCE code verifier and challenge
2. App opens the browser to the OIDC provider's authorize endpoint with:
   - `code_challenge` and `code_challenge_method=S256`
   - A custom redirect URI (e.g. `know://auth/callback`)
3. User authenticates in the browser
4. Browser redirects back to the app with an authorization code
5. App exchanges the code + verifier via `POST /auth/token`:

```bash
curl -X POST http://localhost:4001/auth/token \
  -d '{"code": "auth-code", "code_verifier": "verifier-string"}'
```

Response:
```json
{
  "token": "kh_...",
  "user": {
    "id": "user:abc123",
    "name": "Jane Doe",
    "email": "jane@example.com"
  }
}
```

## Self-Signup

When `KNOW_SELF_SIGNUP_ENABLED=true`, OIDC login automatically creates a new user account if no matching user exists. The matching strategy:

1. **OIDC subject match**: look up by `(provider, subject)` -- exact identity match
2. **Email match**: look up by email -- links the OIDC identity to the existing user
3. **Self-signup**: if enabled, create a new user with the OIDC identity

When disabled (default), only pre-existing users can log in via OIDC. An admin must create the user account first (e.g. via `know db seed`).

## Environment Variables

| Variable                       | Default | Description                                     |
|--------------------------------|---------|-------------------------------------------------|
| `KNOW_OIDC_ENABLED`           | `false` | Enable OIDC authentication                      |
| `KNOW_OIDC_PROVIDER_TYPE`     | `oidc`  | Provider type: `oidc` (standard OIDC) or `github` |
| `KNOW_OIDC_ISSUER_URL`        | —       | OIDC discovery URL (required for `oidc` type, unused for `github`) |
| `KNOW_OIDC_CLIENT_ID`         | —       | OAuth2 client ID                                |
| `KNOW_OIDC_CLIENT_SECRET`     | —       | OAuth2 client secret                            |
| `KNOW_OIDC_REDIRECT_URL`      | —       | Callback URL (e.g. `https://host/auth/callback`)|
| `KNOW_OIDC_PROVIDER_NAME`    | —       | Explicit provider name for DB key (default: derived from issuer URL) |
| `KNOW_SELF_SIGNUP_ENABLED`    | `false` | Auto-create users on first OIDC login           |
| `KNOW_TOKEN_MAX_LIFETIME_DAYS`| `90`    | Max token lifetime in days (0 = no limit)       |
| `KNOW_TOKEN`                  | —       | API token for CLI commands                      |
| `KNOW_TRUST_X_FORWARDED_FOR` | `true`  | Trust X-Forwarded-For header for client IP      |
| `KNOW_RATE_LIMIT_AUTH_RPS`   | `5`     | Requests/sec for `/auth/*` endpoints (0 = off)  |
| `KNOW_RATE_LIMIT_AUTH_BURST` | `10`    | Burst size for auth rate limiter                |
| `KNOW_RATE_LIMIT_GLOBAL_RPS` | `100`   | Requests/sec global per-IP (0 = off)            |
| `KNOW_RATE_LIMIT_GLOBAL_BURST`| `200`  | Burst size for global rate limiter              |

## Rate Limiting

Two tiers of per-IP rate limiting protect the server from abuse:

- **Auth tier** (`/auth/*` endpoints): Stricter limits on unauthenticated login/device-flow/token-exchange endpoints. Default: 5 req/s with burst of 10.
- **Global tier** (all endpoints): A generous global limit applied to every request. Default: 100 req/s with burst of 200.

Set any RPS value to `0` to disable that tier. When a request is rate-limited, the server returns `429 Too Many Requests` with a `Retry-After` header and an RFC 9457 problem detail body.

Rate-limited requests are tracked via the `know_rate_limit_rejected_total` Prometheus counter (label: `tier`).

## Token Limits

- **Max tokens per user**: 50. Attempting to create more returns `409 Conflict`.
- **Self-deletion guard**: You cannot delete the token you are currently authenticating with (returns `409 Conflict`). Token rotation is allowed on the current token.
- **Expired token cleanup**: Expired tokens are automatically removed every 5 minutes.

## Proxy Trust

When deployed behind a reverse proxy (nginx, Traefik, etc.), set `KNOW_TRUST_X_FORWARDED_FOR=true` (default) to use the client IP from the `X-Forwarded-For` header. Set to `false` when the server is directly exposed to use `RemoteAddr` instead, preventing IP spoofing.

## Admin User Management

System admins can create and list users via the CLI. Created users get a private vault automatically.

### CLI Commands

```bash
# Create a user (they can log in via OIDC using their email)
know admin create-user --name alice --email alice@example.com

# List all users
know admin list-users
```

### Private vs Shared Vaults

- **Private vault**: Has an `owner` field set to a user. Created automatically when a user is provisioned (via admin CLI or OIDC self-signup).
- **Shared vault**: Has no `owner`. Created via `db seed` or manually. Any user can be granted membership.

### User Onboarding Flow

1. Admin creates user: `know admin create-user --name alice --email alice@example.com`
2. User logs in via OIDC: `know auth login` → email-match links their OIDC identity
3. User has access to their private vault immediately

When self-signup is enabled (`KNOW_SELF_SIGNUP_ENABLED=true`), new OIDC users are automatically provisioned with a private vault on first login.

## Example Prompts

- "List my API tokens"
- "Create a new token called ci-deploy that expires in 30 days"
- "Rotate my oldest token"
- "Set up OIDC with GitHub for my Know server"
- "How do I log in from the CLI?"
- "Enable self-signup so new users can register via GitHub"
- "Create a new user alice with email alice@company.com"
- "List all users on the server"

## Reference

- OIDC provider: `internal/oidc/` (provider, device flow, PKCE, user resolution)
- Auth endpoints: `internal/api/auth.go` (device start/poll, login/callback, token exchange)
- Admin endpoints: `internal/api/admin.go` (list users, create user)
- Token management: `internal/api/tokens.go` (list, create, delete, rotate)
- CLI auth: `cmd/know/cmd_auth.go` (login, status, logout)
- CLI admin: `cmd/know/cmd_admin.go` (create-user, list-users)
- Token validation: `internal/auth/` (token format, hashing, context)
- Config: `internal/config/config.go` (OIDC and token settings)
