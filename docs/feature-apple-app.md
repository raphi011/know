# Apple App (iPhone, iPad, Mac)

A native SwiftUI app that runs on iPhone, iPad, and Mac from a single codebase.

## Architecture

### Single Multiplatform Target

The app uses XcodeGen's `supportedDestinations: [iOS, macOS]` to compile one target for all platforms. There are no separate iOS/macOS targets or Swift Packages — just `#if os()` guards where platform APIs differ.

**Why this works:** SwiftUI's `NavigationSplitView` automatically adapts per platform:
- **iPhone** — collapses to stack navigation (push to detail, back to list)
- **iPad** — side-by-side split view (sidebar + detail)
- **Mac** — native window with persistent sidebar, toolbar, and traffic lights

### Platform-Specific Code

Only ~5 modifiers need `#if os(iOS)` guards — these are iOS-only APIs that don't exist on macOS:

| Modifier | File | Reason |
|----------|------|--------|
| `.textInputAutocapitalization(.never)` | LoginView.swift | No software keyboard on Mac |
| `.keyboardType(.URL)` | LoginView.swift | No software keyboard on Mac |
| `.navigationBarTitleDisplayMode(.inline)` | DocumentView.swift | No navigation bar on Mac |

Mac-specific additions:
- `.defaultSize(width: 900, height: 600)` on the WindowGroup (KnowApp.swift)
- Separate `Know-macOS.entitlements` with `com.apple.security.network.client`

### Code Sharing

| Layer | Files | Shared? |
|-------|-------|---------|
| Models | 5 | 100% |
| ViewModels | 1 | 100% |
| Networking | 2 | 100% |
| Services | 3 | 100% |
| Utilities | 2 | 100% (Security framework, fuzzy match) |
| Views | 8 | ~95% (5 `#if os` guards) |
| Project config | 2 | Per-platform entitlements |

### Project Setup

The project uses [XcodeGen](https://github.com/yonaskolb/XcodeGen) to generate the Xcode project from `project.yml`:

```yaml
targets:
  Know:
    type: application
    supportedDestinations: [iOS, macOS]
    # ...
```

Key settings:
- **Deployment targets**: iOS 18.0, macOS 15.0
- **Swift version**: 6.0
- **Bundle ID**: `com.know.app` (shared across platforms)
- **Entitlements**: `Know.entitlements` (iOS), `Know-macOS.entitlements` (macOS) via `CODE_SIGN_ENTITLEMENTS[sdk=macosx*]`

### Building

```bash
# Generate Xcode project
cd ios && xcodegen generate

# Build for Mac
xcodebuild -scheme Know -destination 'platform=macOS' build

# Build for iOS Simulator
xcodebuild -scheme Know -destination 'platform=iOS Simulator,name=iPhone 17 Pro' build

# Run on Mac (debug, no code signing)
xcodebuild -scheme Know -destination 'platform=macOS' -derivedDataPath build build \
  CODE_SIGN_IDENTITY=- CODE_SIGNING_REQUIRED=NO CODE_SIGNING_ALLOWED=NO
open build/Build/Products/Debug/Know.app
```

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

## File Structure

```
ios/
├── KnowApp.swift                    # App entry point, WindowGroup, SwiftData container
├── project.yml                      # XcodeGen config (generates .xcodeproj)
├── Know.entitlements                # iOS entitlements (keychain)
├── Know-macOS.entitlements          # macOS entitlements (keychain + network)
├── Models/
│   ├── Document.swift               # Document, SearchResult, ChunkMatch
│   ├── Vault.swift                  # Vault, FileEntry
│   ├── CachedDocument.swift         # SwiftData models (CachedDocument, SyncState)
│   ├── RecentDocument.swift         # Recently viewed document tracking
│   └── Loadable.swift               # Generic loading state wrapper
├── ViewModels/
│   └── QuickPickerViewModel.swift   # Quick picker search and selection logic
├── Networking/
│   ├── RESTClient.swift             # Actor-based HTTP client with auth
│   └── APIError.swift               # Error types
├── Services/
│   ├── AuthService.swift            # Login, logout, session restore
│   ├── KnowService.swift            # High-level API wrapper
│   └── SyncEngine.swift             # Metadata sync, content fetch, SSE streaming
├── Utilities/
│   ├── Keychain.swift               # Security framework wrapper
│   └── FuzzyMatch.swift             # Fuzzy string matching for quick picker
├── Tests/
│   ├── KnowTests.swift              # General tests
│   └── FuzzyMatchTests.swift        # Fuzzy match unit tests
└── Views/
    ├── LoginView.swift              # Server URL + token form
    ├── MainSplitView.swift          # NavigationSplitView (sidebar + detail)
    ├── DocumentView.swift           # Markdown rendering with MarkdownUI
    ├── QuickPickerView.swift        # Quick file picker (⌘K / ⌘P style)
    ├── QuickPickerRow.swift         # Quick picker result row
    └── Components/
        ├── DocumentRow.swift        # Document list cell
        └── SearchResultRow.swift    # Search result display
```

## Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| [MarkdownUI](https://github.com/gonzalezreal/swift-markdown-ui) | 2.4.1 | Markdown rendering in SwiftUI |

## Adding a New Platform-Specific Feature

1. Use `#if os(iOS)` / `#if os(macOS)` for platform-specific code
2. Keep shared logic in Services/Models — only Views should have platform guards
3. Test on both platforms: `xcodebuild -destination 'platform=macOS'` and `xcodebuild -destination 'platform=iOS Simulator,...'`
