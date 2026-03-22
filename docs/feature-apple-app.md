# Apple App (iPhone, iPad, Mac)

A native SwiftUI app that runs on iPhone, iPad, and Mac from a single codebase.

## Technical Reference

For architecture, file structure, build commands, and dependencies, see [tech-apple-app.md](tech-apple-app.md).

## Networking

### REST Client

The app communicates with the Know server via REST API (`RESTClient.swift`). No GraphQL.

| App method | Server endpoint |
|------------|----------------|
| `fetchVaults()` | `GET /api/vaults` |
| `fetchDocument(vaultId, path)` | `GET /api/documents?vault=&path=` |
| `listFiles(vaultId)` | `GET /api/ls?vault=&recursive=true` |
| `search(vaultId, query)` | `GET /api/search?vault=&query=` |
| Auth validation | `GET /api/vaults` (success = valid token) |

### Sync Strategy

1. **Initial sync**: `GET /api/ls?vault=&recursive=true` fetches all file paths, populates SwiftData cache
2. **On-demand content**: document body fetched via `GET /api/documents` when user opens it
3. **Real-time updates**: SSE stream at `GET /events?vaultId=` for `file.created`, `file.updated`, `file.deleted`, `file.moved`, and `file.processed` events
4. **Reconnect recovery**: On SSE reconnect, calls `GET /api/vaults/{vault}/changes?since=<lastSyncedAt>` for incremental catch-up. Falls back to full metadata sync if incremental sync fails.
5. **Offline support**: SwiftData caches document metadata and content for offline browsing

### Auth Flow

1. User enters server URL + API token
2. App validates by calling `GET /api/vaults` with Bearer token
3. Credentials stored in Keychain (`Security` framework — works on both iOS and macOS)
4. Session restored on app launch from Keychain

## Adding a New Platform-Specific Feature

1. Use `#if os(iOS)` / `#if os(macOS)` for platform-specific code
2. Keep shared logic in Services/Models — only Views should have platform guards
3. Test on both platforms: `xcodebuild -destination 'platform=macOS'` and `xcodebuild -destination 'platform=iOS Simulator,...'`
