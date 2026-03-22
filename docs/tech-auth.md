# Auth Architecture

Technical reference for Know's authentication and authorization system. For user-facing setup and CLI usage, see [feature-auth.md](feature-auth.md).

## Overview

Know authenticates all API requests via `kh_`-prefixed bearer tokens. Tokens are issued either directly (bootstrap, API) or through OIDC login flows (device flow, OAuth PKCE). Authorization is vault-scoped: each token inherits its user's vault memberships and roles.

```
                         ┌──────────────────────┐
                         │   OIDC Provider       │
                         │ (GitHub, Google, etc.) │
                         └──────────┬───────────┘
                                    │ ExchangeCode
                                    ▼
┌─────────────┐    ┌──────────────────────────────────┐
│ Claude Code  │    │         Auth Endpoints            │
│ Cursor, CLI  │───▶│  /auth/* (device, PKCE)           │
│ Native Apps  │    │  /oauth/* (DCR, authorize, token) │
└──────┬──────┘    └──────────────┬───────────────────┘
       │                          │ FindOrCreateUser
       │                          │ GenerateToken
       │                          ▼
       │               ┌────────────────────┐
       │               │   kh_ API Token    │
       │               │  (SHA256 → DB)     │
       │               └────────┬───────────┘
       │ Authorization: Bearer  │
       ▼                        ▼
┌──────────────────────────────────────────────────────────┐
│                    Auth Middleware                         │
│  Bearer token → SHA256 → DB lookup → expiry check         │
│  → user lookup → vault memberships → AuthContext           │
└──────────────────────────┬───────────────────────────────┘
                           │
                           ▼
┌──────────────────────────────────────────────────────────┐
│                   Vault Scope Middleware                   │
│  vault name → DB lookup → CheckVaultRole → WithVaultID    │
└──────────────────────────┬───────────────────────────────┘
                           │
                           ▼
                     Route Handler
```

### Request middleware stack

```
HTTP Request
  → Global rate limiter (100 RPS per IP)
    → Request logging (request_id, timing)
      → Security headers
        → Auth middleware (token validation → AuthContext)
          → Vault scope (name → ID, role check)
            → Handler
```

## Token format and lifecycle

### Format

```
kh_<64 hex chars>
│    └── 32 random bytes, hex-encoded
└── prefix (identifies Know tokens)
```

Total length: 67 characters. The prefix allows scanning logs and configs for leaked tokens.

### Generation and storage

```
GenerateToken()
  → rand.Read(32 bytes)
  → raw = "kh_" + hex(bytes)          // shown once to user
  → hash = SHA256(raw) → hex           // stored in DB
  → return (raw, hash)
```

The raw token is **never persisted**. Only the SHA256 hash is stored in the `api_token` table with a UNIQUE index for O(1) lookup.

### Validation flow

`ValidateToken(ctx, dbClient, rawToken)` in `internal/auth/validate.go`:

1. `HashToken(rawToken)` → SHA256 hex digest
2. `GetTokenByHash(hash)` → DB lookup (indexed)
3. `token.IsExpired()` → compare `expires_at` to `time.Now()`
4. `GetUser(userID)` → fetch `is_system_admin` flag
5. `GetVaultMemberships(userID)` → `[]VaultMember` with vault IDs and roles
6. Parse each membership's role string → `VaultRole` enum
7. Safety net: if memberships exist but all fail to parse → error (not silent zero-access)
8. Return `TokenInfo{UserID, IsSystemAdmin, Vaults, TokenID, TokenName}`

After validation, `UpdateTokenLastUsed(tokenID)` fires in a background goroutine (5s timeout, non-blocking).

### Expiry and cleanup

- Expiry is checked at validation time (not proactively)
- `DeleteExpiredTokens()` runs periodically via server cleanup ticker
- Max lifetime: `KNOW_TOKEN_MAX_LIFETIME_DAYS` (default 90, 0 = no limit)
- Per-user limit: 50 tokens (enforced at creation, returns 409)

### Rotation

`RotateToken(oldID, userID, newHash, name, expiresAt)` runs as an atomic DB transaction: creates the new token and deletes the old one. Preserves the remaining TTL (does not extend lifetime). The current token can rotate itself (unlike deletion, which is blocked for the authenticating token).

### Key functions

| Function | File | Purpose |
|----------|------|---------|
| `GenerateToken()` | `internal/auth/token.go` | Create new token + hash |
| `HashToken(raw)` | `internal/auth/token.go` | SHA256 hex digest |
| `UseToken(raw)` | `internal/auth/token.go` | Validate format + hash |
| `ValidateToken(ctx, db, raw)` | `internal/auth/validate.go` | Full validation pipeline |
| `Authenticate(ctx, db, raw, noAuth)` | `internal/auth/validate.go` | ValidateToken + build AuthContext |

## Auth middleware

### Production middleware

`Middleware(dbClient, metrics, config)` in `internal/auth/middleware.go`:

1. Extract `Authorization: Bearer <token>` header
2. If missing → 401 with `WWW-Authenticate` header, audit log `auth.failure`
3. Call `Authenticate(ctx, dbClient, rawToken, false)`
4. If `ErrInvalidToken` → 401, audit `auth.failure`
5. If `ErrTokenExpired` → 401, audit `auth.expired`
6. On success → audit `auth.success`, inject `AuthContext` via `WithAuth(ctx, ac)`, enrich logger with `user_id`, `token_name`

### WWW-Authenticate header (RFC 9728)

When `ResourceMetadataURL` is configured (set by `OAuthHandler.ResourceMetadataURL()`), 401 responses include:

```
WWW-Authenticate: Bearer resource_metadata="https://know.example.com/.well-known/oauth-protected-resource"
```

This tells OAuth-capable clients (e.g. Claude Code) where to discover the authorization server for the `/mcp` resource. Built once at middleware creation time.

### No-auth middleware

`NoAuthMiddleware` in `internal/auth/middleware.go` injects an admin `AuthContext` with wildcard vault access:

```go
AuthContext{
    UserID:        "admin",
    IsSystemAdmin: true,
    Vaults:        [{VaultID: "*", Role: RoleAdmin}],
    Provider:      ProviderNoAuth,
}
```

Activated via `--no-auth` flag or `KNOW_NO_AUTH=true`. Localhost binding only (safety measure in `cmd_serve.go`).

### Audit logging

`AuditLog(ctx, event, attrs...)` in `internal/auth/audit.go` logs at Info level via the context logger. Events:

| Event | Result | When |
|-------|--------|------|
| `auth.success` | `ok` | Valid token, user resolved |
| `auth.failure` | `denied` | Missing header, invalid token |
| `auth.expired` | `expired` | Token found but past `expires_at` |

Fields: `event`, `result`, `user_id`, `ip`, `provider`, `token_name`, `reason`.

## AuthContext and vault RBAC

### AuthContext

```go
type AuthContext struct {
    UserID        string                   // bare user ID (e.g. "admin")
    IsSystemAdmin bool                     // bypasses all role checks
    Vaults        []models.VaultPermission // [{VaultID, Role}, ...]
    Provider      AuthProvider             // "token", "noauth", "oidc"
    TokenID       string                   // bare token ID (for last_used)
    TokenName     string                   // human-readable (e.g. "bootstrap", "oauth-mcp-login")
}
```

Stored in request context via `WithAuth(ctx, ac)`, retrieved via `FromContext(ctx)`.

### Vault roles

Three roles with hierarchical checking:

| Role | Level | Grants |
|------|-------|--------|
| `read` | 1 | Browse documents, search, list |
| `write` | 2 | Create/edit documents, manage memories |
| `admin` | 3 | Vault settings, member management |

`role.AtLeast(required)` checks `role.Level() >= required.Level()`.

Wildcard access: `VaultPermission{VaultID: "*", Role: RoleAdmin}` grants admin on all vaults (no-auth mode only).

### Context helpers

| Function | Purpose |
|----------|---------|
| `WithAuth(ctx, ac)` | Store AuthContext |
| `FromContext(ctx)` | Retrieve AuthContext (error if missing) |
| `WithVaultID(ctx, id)` | Store resolved vault ID |
| `VaultIDFromCtx(ctx)` | Retrieve vault ID (empty if missing) |
| `MustVaultIDFromCtx(ctx)` | Retrieve vault ID (panics if missing) |
| `RequireVaultRole(ctx, vaultID, minRole)` | Check auth + role in one call |
| `CheckVaultRole(ac, vaultID, minRole)` | Pure role check on AuthContext |
| `RequireSystemAdmin(ctx)` | Enforce `is_system_admin` flag |
| `ResolveVault(ctx, ac, svc, name)` | Lookup by name + check access |
| `DetachContext(ctx)` | Copy auth to background context |

### Vault scope middleware

`vaultScope` in `internal/api/vault_scope.go` wraps REST handlers that have a `{vault}` path parameter:

1. Extract vault name from URL path
2. `FromContext(ctx)` → get AuthContext
3. `VaultService.GetByName(name)` → resolve vault record
4. `CheckVaultRole(ac, vaultID, RoleRead)` → 403 if denied
5. `WithVaultID(ctx, vaultID)` → inject for downstream handlers
6. Enrich logger with `vault_id`

### MCP vault resolution

MCP tools use a different path via `resolveVaultIDs()` in `internal/mcptools/auth.go`:

1. Get `AuthContext` from context
2. If wildcard access → fetch all vaults from DB
3. Otherwise → use vault IDs from `ac.Vaults`
4. Filter by `vault.IsMCPEnabled()` (default: true)
5. Return filtered vault ID list

Read tools iterate all accessible vaults. Write tools use the first vault in the list.

**Known issue:** If the user has no vault memberships (empty `ac.Vaults`), all MCP tools silently return "No results found" — indistinguishable from an empty vault. See [Troubleshooting](#troubleshooting).

## OIDC provider layer

### Provider interface

```go
type Provider interface {
    ProviderName() string
    AuthCodeURL(state string) string
    ExchangeCode(ctx context.Context, code string) (*UserInfo, error)
}
```

`UserInfo` carries: `Provider`, `Subject` (stable ID), `Email`, `Name`, `Login` (username fallback).

### Standard OIDC provider

`internal/oidc/provider.go` — uses `go-oidc/v3` for discovery and ID token verification.

- Queries `/.well-known/openid-configuration` during init
- Verifies ID tokens with provider's JWKS
- Derives provider name from issuer URL (e.g. `https://accounts.google.com` → `accounts.google.com`)
- Override via `KNOW_OIDC_PROVIDER_NAME`

### GitHub provider

`internal/oidc/github.go` — GitHub doesn't support standard OIDC (no ID tokens, no discovery).

- Uses GitHub's OAuth2 endpoints directly
- Fetches user info via REST API: `GET /user` + `GET /user/emails`
- Email selection: primary+verified > any verified > first unverified
- Uses numeric GitHub user ID as `Subject` (stable, unlike login which can change)
- PKCE limitation: GitHub OAuth Apps silently ignore `code_verifier`

## User resolution

`FindOrCreateUser()` in `internal/oidc/user.go` resolves an OIDC identity to a Know user:

```
1. GetUserByOIDCSubject(provider, subject)
   → found → return existing user

2. GetUserByEmail(email)
   → found → LinkOIDCIdentity(userID, provider, subject)
   → return existing user (no vault provisioning)

3. selfSignup enabled?
   → yes → ProvisionUserFromOIDC(provider, subject, name, email)
           creates: user + private vault + admin membership
   → no  → error: "registration disabled"
```

**Important:** Path 2 (email match) links the OIDC identity but does NOT create vault memberships. It assumes the matched user already has appropriate memberships. If the bootstrap user was created without an email, path 2 cannot match, and path 3 will either create a new user (self-signup) or fail (no self-signup).

### Provisioning

`ProvisionUserFromOIDC()` in `internal/db/queries_user.go`:

1. `CreateUserFromOIDC(provider, subject, name, email)` → new user record
2. `CreateVaultWithOwner(userID, {Name: user.Name})` → private vault
3. `CreateVaultMember(userID, vaultID, RoleAdmin)` → admin membership

The admin CLI (`know admin create-user`) follows the same provisioning path via `ProvisionUser()`.

## Auth flows

### Device flow (CLI login)

Used by `know auth login` for browser-based OIDC authentication.

```
CLI                          Server                      OIDC Provider
 │                             │                              │
 │  POST /auth/device/start    │                              │
 │────────────────────────────▶│                              │
 │  {user_code, device_code,   │                              │
 │   verification_uri}         │                              │
 │◀────────────────────────────│                              │
 │                             │                              │
 │  [display user_code,        │                              │
 │   open browser]             │                              │
 │                             │                              │
 │         User visits verification_uri in browser            │
 │                             │                              │
 │                             │  GET /auth/login?user_code=  │
 │                             │  → sign state(HMAC)          │
 │                             │  → redirect to provider      │
 │                             │─────────────────────────────▶│
 │                             │                              │
 │                             │  GET /auth/callback           │
 │                             │  state = user_code.sig        │
 │                             │◀─────────────────────────────│
 │                             │                              │
 │                             │  ExchangeCode → UserInfo     │
 │                             │  FindOrCreateUser            │
 │                             │  GenerateToken → kh_...      │
 │                             │  EncryptWithSecret(token,    │
 │                             │    device_code)              │
 │                             │  Store in device_code record │
 │                             │                              │
 │  POST /auth/device/poll     │                              │
 │  {device_code}              │                              │
 │────────────────────────────▶│                              │
 │  {token (encrypted)}        │                              │
 │◀────────────────────────────│                              │
 │                             │                              │
 │  DecryptWithSecret(token,   │                              │
 │    device_code)             │                              │
 │  Save to system keychain    │                              │
```

- User code: 8 uppercase letters, displayed as `XXXX-XXXX`
- Device code: 32 random bytes (64 hex chars)
- Expiry: 15 minutes
- Poll interval: 5 seconds
- State parameter: `user_code.HMAC(user_code, state_secret)`
- Token encryption: AES-256-GCM with `SHA256(device_code)` as key

### OAuth MCP flow (Claude Code, Cursor)

Used when MCP clients connect via `claude mcp add --transport http`.

```
MCP Client                   Server                      OIDC Provider
 │                             │                              │
 │  GET /.well-known/          │                              │
 │  oauth-protected-resource   │                              │
 │────────────────────────────▶│                              │
 │  {resource, auth_servers}   │                              │
 │◀────────────────────────────│                              │
 │                             │                              │
 │  GET /.well-known/          │                              │
 │  oauth-authorization-server │                              │
 │────────────────────────────▶│                              │
 │  {issuer, endpoints, ...}   │                              │
 │◀────────────────────────────│                              │
 │                             │                              │
 │  POST /oauth/register       │     (RFC 7591 DCR)          │
 │  {client_name, redirect_uris}                              │
 │────────────────────────────▶│                              │
 │  {client_id}                │                              │
 │◀────────────────────────────│                              │
 │                             │                              │
 │  Generate PKCE pair         │                              │
 │  (code_verifier, challenge) │                              │
 │                             │                              │
 │  GET /oauth/authorize       │                              │
 │  ?client_id=...             │                              │
 │  &code_challenge=...        │                              │
 │  &redirect_uri=...          │                              │
 │  &state=client_state        │                              │
 │────────────────────────────▶│                              │
 │                             │  Sign OAuth state (HMAC)     │
 │                             │  Redirect to provider        │
 │                             │─────────────────────────────▶│
 │                             │                              │
 │                             │  GET /auth/callback           │
 │                             │  state = oauth:payload.sig    │
 │                             │◀─────────────────────────────│
 │                             │                              │
 │                             │  VerifyOAuthState             │
 │                             │  ExchangeCode → UserInfo     │
 │                             │  FindOrCreateUser            │
 │                             │  GenerateToken → kh_...      │
 │                             │  EncryptWithSecret(token,    │
 │                             │    code_challenge)           │
 │                             │  Store as oauth_auth_code    │
 │                             │  Redirect to redirect_uri    │
 │                             │    ?code=auth_code           │
 │                             │    &state=client_state       │
 │                             │                              │
 │  POST /oauth/token          │                              │
 │  grant_type=authorization_code                             │
 │  code=auth_code             │                              │
 │  code_verifier=...          │                              │
 │────────────────────────────▶│                              │
 │                             │  ConsumeOAuthAuthCode (atomic)│
 │                             │  Verify PKCE:                │
 │                             │    SHA256(verifier) == challenge│
 │                             │  DecryptWithSecret(token,    │
 │                             │    code_challenge)           │
 │  {access_token, token_type} │                              │
 │◀────────────────────────────│                              │
 │                             │                              │
 │  Store in system keychain   │                              │
```

Key properties:
- Discovery: RFC 9728 (Protected Resource Metadata) → RFC 8414 (Authorization Server Metadata)
- Registration: RFC 7591 (Dynamic Client Registration) — loopback redirect URIs only
- PKCE: S256 only (plain rejected)
- Auth code: one-time use via atomic `DELETE ... RETURN BEFORE` in SurrealDB
- Token encryption: AES-256-GCM with `SHA256(code_challenge)` as key
- Auth code expiry: 60 seconds

### Native app PKCE flow (iOS, macOS)

Used by the Know iOS/macOS app:

1. App generates PKCE pair (`code_verifier`, `code_challenge`)
2. Opens browser to OIDC provider's authorize endpoint with challenge and custom redirect URI (`know://auth/callback`)
3. User authenticates in browser
4. Browser redirects back to app with authorization code
5. App exchanges code + verifier via `POST /auth/token`
6. Returns `{token, user: {id, name, email}}`

This flow uses the OIDC provider directly (not the OAuth AS facade).

## OAuth Authorization Server facade

`internal/api/oauth.go` — implements an OAuth 2.0 AS that proxies to the upstream OIDC provider.

### Endpoints

| Method | Path | Purpose | RFC |
|--------|------|---------|-----|
| GET | `/.well-known/oauth-authorization-server` | AS metadata | RFC 8414 |
| GET | `/.well-known/oauth-protected-resource` | Resource metadata for `/mcp` | RFC 9728 |
| POST | `/oauth/register` | Dynamic Client Registration | RFC 7591 |
| GET | `/oauth/authorize` | Start auth flow (redirect to OIDC) | |
| POST | `/oauth/token` | Exchange auth code + PKCE for token | |

All endpoints are **unauthenticated** (registered before the auth middleware) and protected by the auth-tier rate limiter.

### Dynamic Client Registration

`handleRegister` accepts `{client_name, redirect_uris[]}`:

- **Redirect URI validation:** Only loopback addresses allowed (`localhost`, `127.0.0.1`, `[::1]`). Prevents phishing via external redirect URIs.
- **Port-agnostic matching:** Per RFC 7591, the port is ignored for loopback URIs since native clients bind to ephemeral ports. Matching compares scheme, hostname, and path only.
- **Client ID:** UUID v4, stored in `oauth_client` table.

### Authorization Server Metadata

```json
{
  "issuer": "https://know.example.com",
  "authorization_endpoint": "https://know.example.com/oauth/authorize",
  "token_endpoint": "https://know.example.com/oauth/token",
  "registration_endpoint": "https://know.example.com/oauth/register",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code"],
  "code_challenge_methods_supported": ["S256"],
  "token_endpoint_auth_methods_supported": ["none"]
}
```

### Protected Resource Metadata

```json
{
  "resource": "https://know.example.com/mcp",
  "authorization_servers": ["https://know.example.com"]
}
```

### OAuth state management

The `/oauth/authorize` endpoint creates an HMAC-signed state parameter that carries the redirect URI, PKCE code challenge, and client state through the OIDC provider redirect:

```
State = "oauth:" + base64url(JSON({redirect_uri, code_challenge, client_state})) + "." + HMAC-SHA256(payload, state_secret)
```

The state secret is 32 random bytes generated at server startup. **Restarting the server invalidates all in-flight OAuth flows.**

The `/auth/callback` endpoint dispatches based on state prefix:
- `"oauth:"` prefix → OAuth MCP flow (`handleOAuthCallback`)
- No prefix → Device flow (`handleDeviceFlowCallback`)

## Database schema

### Auth-related tables

```sql
-- User
DEFINE TABLE user SCHEMAFULL;
DEFINE FIELD name             ON user TYPE string;
DEFINE FIELD email            ON user TYPE option<string>;
DEFINE FIELD is_system_admin  ON user TYPE bool DEFAULT false;
DEFINE FIELD created_at       ON user TYPE datetime DEFAULT time::now();
DEFINE FIELD oidc_provider    ON user TYPE option<string>;
DEFINE FIELD oidc_subject     ON user TYPE option<string>;
DEFINE INDEX idx_user_name ON user FIELDS name UNIQUE;
DEFINE INDEX idx_user_oidc ON user FIELDS oidc_provider, oidc_subject UNIQUE;

-- API Token
DEFINE TABLE api_token SCHEMAFULL;
DEFINE FIELD user         ON api_token TYPE record<user>;
DEFINE FIELD token_hash   ON api_token TYPE string;
DEFINE FIELD name         ON api_token TYPE string;
DEFINE FIELD last_used    ON api_token TYPE option<datetime>;
DEFINE FIELD expires_at   ON api_token TYPE option<datetime>;
DEFINE FIELD created_at   ON api_token TYPE datetime DEFAULT time::now();
DEFINE INDEX idx_api_token_hash ON api_token FIELDS token_hash UNIQUE;

-- Vault Member (RBAC)
DEFINE TABLE vault_member SCHEMAFULL;
DEFINE FIELD user       ON vault_member TYPE record<user>;
DEFINE FIELD vault      ON vault_member TYPE record<vault>;
DEFINE FIELD role       ON vault_member TYPE string
    ASSERT $value IN ["read", "write", "admin"];
DEFINE FIELD created_at ON vault_member TYPE datetime DEFAULT time::now();
DEFINE INDEX idx_vault_member_user_vault ON vault_member FIELDS user, vault UNIQUE;
DEFINE INDEX idx_vault_member_vault      ON vault_member FIELDS vault;

-- Device Code (device authorization grant)
DEFINE TABLE device_code SCHEMAFULL;
DEFINE FIELD user_code    ON device_code TYPE string;
DEFINE FIELD device_code  ON device_code TYPE string;
DEFINE FIELD expires_at   ON device_code TYPE datetime;
DEFINE FIELD user         ON device_code TYPE option<record<user>>;
DEFINE FIELD approved     ON device_code TYPE bool DEFAULT false;
DEFINE FIELD raw_token    ON device_code TYPE option<string>;
DEFINE FIELD created_at   ON device_code TYPE datetime DEFAULT time::now();
DEFINE INDEX idx_device_code_code      ON device_code FIELDS device_code UNIQUE;
DEFINE INDEX idx_device_code_user_code ON device_code FIELDS user_code UNIQUE;

-- OAuth Client (RFC 7591 DCR)
DEFINE TABLE oauth_client SCHEMAFULL;
DEFINE FIELD client_id     ON oauth_client TYPE string;
DEFINE FIELD client_name   ON oauth_client TYPE string;
DEFINE FIELD redirect_uris ON oauth_client TYPE array<string>;
DEFINE FIELD created_at    ON oauth_client TYPE datetime DEFAULT time::now();
DEFINE INDEX idx_oauth_client_id ON oauth_client FIELDS client_id UNIQUE;

-- OAuth Auth Code (authorization code grant)
DEFINE TABLE oauth_auth_code SCHEMAFULL;
DEFINE FIELD code            ON oauth_auth_code TYPE string;
DEFINE FIELD encrypted_token ON oauth_auth_code TYPE string;
DEFINE FIELD code_challenge  ON oauth_auth_code TYPE string;
DEFINE FIELD redirect_uri    ON oauth_auth_code TYPE string;
DEFINE FIELD expires_at      ON oauth_auth_code TYPE datetime;
DEFINE FIELD created_at      ON oauth_auth_code TYPE datetime DEFAULT time::now();
DEFINE INDEX idx_oauth_auth_code ON oauth_auth_code FIELDS code UNIQUE;
```

### Entity relationships

```
user ──1:N──▶ api_token       (user owns tokens)
user ──1:N──▶ vault_member    (user has vault memberships)
vault ──1:N──▶ vault_member   (vault has members)
user ──1:N──▶ device_code     (user approves device flows)
```

### Cascade events

- Deleting a vault automatically deletes its `vault_member` records
- Tokens and device codes are cleaned up by periodic background tasks

## Security properties

### Token security

- **Raw token never stored:** Only SHA256 hash persists in DB
- **Token prefix (`kh_`):** Enables automated secret scanning in logs and repos
- **Indexed hash lookup:** UNIQUE index on `token_hash` for O(1) validation

### Cryptographic primitives

| Primitive | Usage | Key derivation |
|-----------|-------|----------------|
| SHA256 | Token hashing, PKCE challenge | Direct hash |
| HMAC-SHA256 | OAuth state signing, device flow state | Per-startup 32-byte secret |
| AES-256-GCM | Token encryption in device/OAuth flows | SHA256(device_code or code_challenge) |
| `crypto/rand` | Token generation, nonces, auth codes | OS entropy |

### OAuth security

- **PKCE enforcement:** S256 only (plain rejected). Prevents authorization code interception.
- **Loopback-only redirect URIs:** DCR rejects non-loopback URIs, preventing phishing via external redirects.
- **Port-agnostic matching:** Redirect URI comparison ignores port (native clients bind ephemeral ports).
- **Atomic auth code consumption:** `DELETE ... RETURN BEFORE` prevents replay attacks.
- **Auth code expiry:** 60 seconds.
- **State secret rotation:** Per-startup random secret invalidates in-flight flows on restart.
- **Token encryption:** The raw `kh_` token is encrypted with the PKCE code challenge before storage. Only the client holding the `code_verifier` can derive the challenge and decrypt.

### Rate limiting

Two tiers of per-IP rate limiting:

| Tier | Endpoints | Default RPS | Default burst |
|------|-----------|-------------|---------------|
| Auth | `/auth/*`, `/oauth/*` | 5 | 10 |
| Global | All endpoints | 100 | 200 |

Set any RPS to 0 to disable. Returns `429 Too Many Requests` with `Retry-After` header and RFC 9457 problem detail body. Tracked via `know_rate_limit_rejected_total` Prometheus counter.

## Helm chart configuration

### values.yaml

```yaml
oidc:
  enabled: false
  providerType: "oidc"           # "oidc" or "github"
  issuerURL: ""                  # required for "oidc" type
  clientID: ""
  clientSecret: ""               # stored in secret.yaml
  redirectURL: ""                # e.g. https://know.example.com/auth/callback
  providerName: ""               # override provider name in DB
  selfSignupEnabled: false       # auto-create users on first OIDC login

rateLimit:
  authRPS: 5
  authBurst: 10
  globalRPS: 100
  globalBurst: 200

trustXForwardedFor: true         # set false when not behind a proxy
```

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOW_NO_AUTH` | `false` | Bypass token auth (dev only, localhost) |
| `KNOW_OIDC_ENABLED` | `false` | Enable OIDC authentication |
| `KNOW_OIDC_PROVIDER_TYPE` | `oidc` | `oidc` or `github` |
| `KNOW_OIDC_ISSUER_URL` | — | OIDC discovery URL |
| `KNOW_OIDC_CLIENT_ID` | — | OAuth2 client ID |
| `KNOW_OIDC_CLIENT_SECRET` | — | OAuth2 client secret |
| `KNOW_OIDC_REDIRECT_URL` | — | Callback URL |
| `KNOW_OIDC_PROVIDER_NAME` | — | Explicit provider name for DB |
| `KNOW_SELF_SIGNUP_ENABLED` | `false` | Auto-create users on OIDC login |
| `KNOW_TOKEN_MAX_LIFETIME_DAYS` | `90` | Max token lifetime (0 = no limit) |
| `KNOW_TRUST_X_FORWARDED_FOR` | `true` | Trust `X-Forwarded-For` for client IP |
| `KNOW_RATE_LIMIT_AUTH_RPS` | `5` | Auth endpoint rate limit |
| `KNOW_RATE_LIMIT_AUTH_BURST` | `10` | Auth endpoint burst |
| `KNOW_RATE_LIMIT_GLOBAL_RPS` | `100` | Global per-IP rate limit |
| `KNOW_RATE_LIMIT_GLOBAL_BURST` | `200` | Global burst |
| `KNOW_TOKEN` | — | API token for CLI commands |
| `KNOW_BOOTSTRAP_USER_NAME` | `admin` | Bootstrap user name |
| `KNOW_BOOTSTRAP_USER_EMAIL` | — | Bootstrap user email (important for OAuth matching) |
| `KNOW_BOOTSTRAP_USER_ID` | `admin` | Bootstrap user record ID |
| `KNOW_BOOTSTRAP_VAULT_ID` | `default` | Bootstrap vault record ID |
| `KNOW_BOOTSTRAP_VAULT_NAME` | `default` | Bootstrap vault display name |
| `KNOW_BOOTSTRAP_TOKEN` | — | Reuse a specific token for bootstrap |

## Bootstrap flow

`know db seed` in `cmd/know/cmd_db_seed.go`:

1. `InitSchema(ctx, embedDim)` — create all tables and indexes
2. `CreateUserWithID("admin", {Name, Email})` — stable user ID
3. `UpdateUserSystemAdmin(userID, true)` — mark as system admin
4. `CreateVaultWithID("default", userID, {Name, Description})` — default vault
5. `CreateVaultMember(userID, "default", RoleAdmin)` — admin membership
6. `CreateToken(userID, hash, "bootstrap", expiry)` — API token
7. Output: raw token (shown once), vault ID

**Important for OAuth:** If `KNOW_BOOTSTRAP_USER_EMAIL` is not set, the bootstrap user has no email. When a user later authenticates via OIDC, `FindOrCreateUser` cannot match by email (path 2), and:
- With self-signup enabled: creates a **new user** with a **new empty vault**
- With self-signup disabled: login fails entirely

Set `KNOW_BOOTSTRAP_USER_EMAIL` to the OIDC user's email to ensure path 2 matches correctly.

## Token Management API

REST endpoints for users to manage their own tokens (authenticated):

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/tokens` | List user's tokens |
| POST | `/api/v1/tokens` | Create token (`{name, expires_in_days}`) |
| DELETE | `/api/v1/tokens/{id}` | Delete token (cannot delete self) |
| POST | `/api/v1/tokens/{id}/rotate` | Atomic replace (preserves TTL) |

Limits: 50 tokens per user, `expires_in_days` capped by server max, self-deletion blocked (409).

## Troubleshooting

### OAuth user has no vault access (MCP returns empty)

**Symptoms:** MCP tools return "No results found", "No folders found", etc. even though vault has documents.

**Cause:** The OAuth-authenticated user has no `vault_member` records for the vault containing documents.

**Diagnosis:**
```sql
-- Find the OAuth token and its user
SELECT id, user, name FROM api_token WHERE name = 'oauth-mcp-login';

-- Check that user's vault memberships
SELECT * FROM vault_member WHERE user = user:$USER_ID;

-- List all users and vaults
SELECT id, name, email FROM user;
SELECT id, name, owner FROM vault;
```

**Fix:** Add the user as a vault member:
```sql
CREATE vault_member SET
    user = user:$OAUTH_USER_ID,
    vault = vault:default,
    role = 'admin';
```

**Prevention:** Set `KNOW_BOOTSTRAP_USER_EMAIL` to match the OIDC user's email so `FindOrCreateUser` path 2 links correctly.

### Token expired

**Symptoms:** 401 responses, audit log shows `auth.expired`.

**Check:** `SELECT id, name, expires_at FROM api_token WHERE user = user:$ID`

### OIDC login fails

**Symptoms:** "registration disabled: no matching user" error.

**Cause:** Self-signup disabled and no matching user found by OIDC subject or email.

**Fix:** Either enable self-signup or pre-create the user: `know admin create-user --name alice --email alice@example.com`

### In-flight OAuth flows fail after server restart

**Cause:** The OAuth state secret is regenerated on each startup. Any OAuth or device flows started before the restart will have invalid state signatures.

**Fix:** User retries the login flow. This is by design (security tradeoff).

## Source files

| File | Purpose |
|------|---------|
| `internal/auth/token.go` | Token format, generation, hashing |
| `internal/auth/validate.go` | Token validation pipeline |
| `internal/auth/context.go` | AuthContext, vault role checking |
| `internal/auth/middleware.go` | Middleware, NoAuthMiddleware |
| `internal/auth/audit.go` | Audit event logging |
| `internal/oidc/user.go` | FindOrCreateUser (3-path strategy) |
| `internal/oidc/provider.go` | OIDC provider interface + standard impl |
| `internal/oidc/github.go` | GitHub OAuth provider |
| `internal/oidc/device.go` | Device code generation, AES encryption |
| `internal/oidc/oauth_state.go` | OAuth state HMAC signing |
| `internal/oidc/pkce.go` | PKCE code verifier/challenge |
| `internal/api/auth.go` | Auth endpoint handlers |
| `internal/api/oauth.go` | OAuth AS facade (RFC 7591, 9728) |
| `internal/api/tokens.go` | Token CRUD API |
| `internal/api/admin.go` | Admin user management |
| `internal/api/vault_scope.go` | Vault scope middleware |
| `internal/mcptools/auth.go` | MCP vault resolution |
| `internal/db/schema.go` | DDL for all auth tables |
| `internal/db/queries_token.go` | Token DB operations |
| `internal/db/queries_user.go` | User DB operations + provisioning |
| `internal/db/queries_vault_member.go` | Membership queries |
| `cmd/know/cmd_serve.go` | Middleware wiring, endpoint registration |
| `cmd/know/cmd_db_seed.go` | Bootstrap flow |
| `cmd/know/cmd_auth.go` | CLI auth commands |
| `helm/know/values.yaml` | Helm chart defaults |
| `helm/know/templates/deployment.yaml` | Env var mapping |
| `helm/know/templates/secret.yaml` | OIDC secrets |
