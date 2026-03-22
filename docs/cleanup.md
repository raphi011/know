# Code Cleanup Backlog

Remaining simplification opportunities identified by code review. Items are ordered by impact.

## Medium Priority

### 1. `NewAuthHandler` has 5 parameters
**File:** `internal/api/auth.go:38`

```go
func NewAuthHandler(provider oidc.Provider, dbClient *db.Client, selfSignup bool, redirectURL string, tokenMaxLifetimeDays int) (*AuthHandler, error)
```

Consolidate into an `AuthHandlerConfig` struct. Single call site in `cmd/know/cmd_serve.go`.

### 2. `file.NewService` has 7 parameters
**File:** `internal/file/service.go:237`

```go
func NewService(db *db.Client, blobStore blob.Store, embedder *llm.Embedder, chunkConfig parser.ChunkConfig, versionConfig VersionConfig, bus *event.Bus, embedMaxInputChars int)
```

Group into a `ServiceConfig` struct. Single call site in `internal/server/bootstrap.go`.

### 3. Bookmark handlers repeat auth extraction
**File:** `internal/api/bookmarks.go`

All three handlers (`listBookmarks`, `addBookmark`, `removeBookmark`) repeat:
```go
ac, err := auth.FromContext(ctx)
if err != nil {
    httputil.WriteProblem(w, http.StatusUnauthorized, "unauthorized")
    return
}
userID := ac.UserID
```

Could extract to a `requireUserID(w, r) (string, bool)` helper in the api package, or use middleware.

## Lower Priority

### 4. `config.Load()` is ~137 lines
**File:** `internal/config/config.go:228`

Massive function reading env vars. Could split by category (DB, LLM, Auth, SSH, etc.) into private helpers like `loadDBConfig()`, `loadAuthConfig()`.

### 5. Multi-vault tool error log duplication
**File:** `internal/tools/multivault.go`

Identical `logger.Warn("multi-vault tool failed", ...)` in 3 methods (`runConcat`, `runFirstHit`, `runDedupCSV`). Extract to a private `logToolError` method.

### 6. Image MIME type validation inline
**File:** `internal/agent/handler.go:88-96`

Switch on `"image/png", "image/jpeg", "image/gif", "image/webp"` could use a `models.IsValidImageMIME()` helper from `internal/models/mime.go`.

### 7. Deep nesting in attachment validation
**File:** `internal/agent/handler.go:79-97`

Attachment validation loop has nested if/switch. Could extract to `validateAttachment()` helper.

### 8. Gemini MIME support duplication
**Files:** `internal/llm/gemini_multimodal_embedder.go:114`, `internal/llm/gemini_text_extractor.go:96`

Near-identical `SupportsMIME()` methods. Acceptable since they're in different subsystems with potentially different future needs, but could share a base set.
