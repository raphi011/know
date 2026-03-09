# Go WebDAV Package Reference (`golang.org/x/net/webdav`)

Technical reference for the Go WebDAV package. Covers API, best practices, performance, security, client quirks, and patterns used in this project.

## Overview

`golang.org/x/net/webdav` provides a DAV Class 1 and 2 compliant WebDAV server. It handles HTTP method dispatch, XML marshalling, property management, and lock coordination — you supply the storage backend via two interfaces: `FileSystem` and `LockSystem`.

**What it does**: PROPFIND, PROPPATCH, MKCOL, GET, HEAD, PUT, DELETE, COPY, MOVE, LOCK, UNLOCK, OPTIONS.

**What it doesn't do**: Authentication, TLS, access control (ACL/RFC 3744), versioning (DeltaV), CalDAV, CardDAV.

## Core Types

### Handler

The main entry point. Implements `http.Handler`.

```go
type Handler struct {
    Prefix     string              // URL path prefix to strip (e.g. "/dav/")
    FileSystem FileSystem          // Required: virtual filesystem
    LockSystem LockSystem          // Required: lock manager
    Logger     func(*http.Request, error) // Optional: called for every request
}
```

- `Prefix` is stripped from `r.URL.Path` before passing to `FileSystem` methods. If your WebDAV endpoint is at `/dav/files/`, set `Prefix: "/dav/files"`.
- `Logger` receives `nil` error on success. Useful for structured logging with severity filtering (see project pattern in `handler.go`).
- Both `FileSystem` and `LockSystem` **must** be set or `ServeHTTP` panics.

### FileSystem

```go
type FileSystem interface {
    Mkdir(ctx context.Context, name string, perm os.FileMode) error
    OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (File, error)
    RemoveAll(ctx context.Context, name string) error
    Rename(ctx context.Context, oldName, newName string) error
    Stat(ctx context.Context, name string) (os.FileInfo, error)
}
```

Semantics match the `os` package. Paths are always `/`-separated regardless of OS.

**Error conventions**:
- Return `os.ErrNotExist` → 404
- Return `os.ErrPermission` → 403
- Return `os.ErrExist` → 405 (for Mkdir on existing)
- Any other error → 500

**Built-in implementations**:
- `Dir(path)` — wraps a native directory, restricts to subtree
- `NewMemFS()` — in-memory filesystem (useful for tests)

### File

```go
type File interface {
    http.File    // Read, Seek, Readdir, Stat, Close
    io.Writer    // Write
}
```

`http.File` embeds `io.Reader`, `io.Seeker`, `io.Closer` + `Readdir` + `Stat`. Adding `io.Writer` means every File must have a `Write` method — return `os.ErrPermission` for read-only files.

**Readdir contract**:
- `count <= 0`: return all remaining entries, `nil` error (or underlying error)
- `count > 0`: return up to `count` entries. Return `io.EOF` when no more entries remain. The handler calls Readdir repeatedly to paginate.

### LockSystem

```go
type LockSystem interface {
    Confirm(now time.Time, name0, name1 string, conditions ...Condition) (release func(), err error)
    Create(now time.Time, details LockDetails) (token string, err error)
    Refresh(now time.Time, token string, duration time.Duration) (LockDetails, error)
    Unlock(now time.Time, token string) error
}
```

**`Confirm`** is called on every state-changing request. It validates that submitted lock tokens match. Returns a `release` func that must be called when the request finishes (the handler defers it). Two resource names support COPY/MOVE (source + destination).

**`Create`** returns an opaque token (must be an absolute URI per RFC 3986 §4.3). `NewMemLS()` generates `urn:uuid:...` tokens.

**Error → HTTP status mapping**:
| Error | Status |
|---|---|
| `ErrConfirmationFailed` | Try next condition set |
| `ErrLocked` | 423 Locked |
| `ErrNoSuchLock` | 412 Precondition Failed |
| `ErrForbidden` | 403 Forbidden |
| Other non-nil error | 500 Internal Server Error |

### LockDetails

```go
type LockDetails struct {
    Root      string        // Resource being locked
    Duration  time.Duration // Negative = infinite
    OwnerXML  string        // Verbatim <owner> XML from LOCK request
    ZeroDepth bool          // true = zero depth, false = infinite depth
}
```

### Dir

```go
type Dir string // e.g. Dir("/var/webdav")
```

A `FileSystem` backed by the native filesystem, restricted to the given directory tree. Empty string means current directory. Handles path sanitization to prevent traversal.

### Optional Interfaces

These are checked on `os.FileInfo` objects returned by `Stat()`:

**`ContentTyper`** — Override MIME type detection:
```go
type ContentTyper interface {
    ContentType(ctx context.Context) (string, error)
}
```
If not implemented, the handler opens the file and sniffs the first 512 bytes via `http.DetectContentType`. Return `ErrNotImplemented` to fall back to sniffing.

**`ETager`** — Override ETag generation:
```go
type ETager interface {
    ETag(ctx context.Context) (string, error)
}
```
If not implemented, ETag is computed from `ModTime()` and `Size()`. Return `ErrNotImplemented` to use default.

**`DeadPropsHolder`** — Store custom (dead) properties:
```go
type DeadPropsHolder interface {
    DeadProps() (map[xml.Name]Property, error)
    Patch([]Proppatch) ([]Propstat, error)
}
```
Dead properties are user-defined XML properties (as opposed to "live" properties like `getcontentlength` which the server computes). Without this interface, PROPPATCH on custom properties returns 403.

### Constants and Sentinel Errors

```go
// HTTP status codes from RFC 4918
const (
    StatusMulti               = 207 // Multi-Status
    StatusUnprocessableEntity = 422
    StatusLocked              = 423
    StatusFailedDependency    = 424
    StatusInsufficientStorage = 507
)

// LockSystem errors
var (
    ErrConfirmationFailed = errors.New("webdav: confirmation failed")
    ErrForbidden          = errors.New("webdav: forbidden")
    ErrLocked             = errors.New("webdav: locked")
    ErrNoSuchLock         = errors.New("webdav: no such lock")
)

// Optional interface fallback
var ErrNotImplemented = errors.New("not implemented")
```

## Handler Configuration

### Routing

The `Handler` dispatches based on HTTP method. Typical setup:

```go
mux := http.NewServeMux()
mux.Handle("/dav/", &webdav.Handler{
    Prefix:     "/dav",
    FileSystem: webdav.Dir("/var/webdav"),
    LockSystem: webdav.NewMemLS(),
})
```

**Important**: The prefix should NOT include a trailing slash. The handler strips `Prefix` from `r.URL.Path`, so `/dav/file.txt` becomes `/file.txt`.

### Method Dispatch

| Method | FileSystem calls | Notes |
|---|---|---|
| OPTIONS | — | Returns Allow header + DAV compliance classes |
| GET/HEAD | OpenFile (read) | Serves file content via `http.ServeContent` |
| PUT | OpenFile (write) | Creates/overwrites file |
| DELETE | RemoveAll | Recursive for directories |
| MKCOL | Mkdir | Only creates one level |
| COPY | Stat + OpenFile (read) + OpenFile (write) | Recursive for directories |
| MOVE | Rename (or copy+delete fallback) | |
| PROPFIND | Stat + OpenFile (for Readdir) | Depth 0, 1, or infinity |
| PROPPATCH | Stat | Modifies dead properties |
| LOCK | LockSystem.Create | |
| UNLOCK | LockSystem.Unlock | |

## Implementing FileSystem

### Path Rules

- Always `/`-separated, always start with `/`
- The handler passes cleaned paths (no `..`, no double slashes)
- Root is `/`

### Error Mapping

Use `os` sentinel errors for proper HTTP status codes:

```go
func (fs *MyFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
    item, err := fs.db.Get(ctx, name)
    if err != nil {
        return nil, fmt.Errorf("stat %s: %w", name, err) // → 500
    }
    if item == nil {
        return nil, os.ErrNotExist // → 404
    }
    return itemToFileInfo(item), nil
}
```

### OpenFile Flags

The `flag` parameter uses `os` constants:
- `os.O_RDONLY` (0) — GET, PROPFIND
- `os.O_RDWR | os.O_CREATE | os.O_TRUNC` — PUT (create/overwrite)
- `os.O_RDWR | os.O_CREATE` — PUT (conditional create with If-None-Match)

Your implementation should inspect these flags to decide read vs write mode.

## Implementing File

### Minimum Viable File

A read-only file backed by `bytes.Reader`:

```go
type readOnlyFile struct {
    name    string
    content []byte
    reader  *bytes.Reader
    modTime time.Time
}

func (f *readOnlyFile) Read(p []byte) (int, error)  { return f.reader.Read(p) }
func (f *readOnlyFile) Seek(o int64, w int) (int64, error) { return f.reader.Seek(o, w) }
func (f *readOnlyFile) Write([]byte) (int, error)   { return 0, os.ErrPermission }
func (f *readOnlyFile) Close() error                 { return nil }
func (f *readOnlyFile) Readdir(int) ([]os.FileInfo, error) { return nil, os.ErrInvalid }
func (f *readOnlyFile) Stat() (os.FileInfo, error) {
    return &fileInfo{name: f.name, size: int64(len(f.content)), modTime: f.modTime}, nil
}
```

### Write-on-Close Pattern

Buffer writes in memory, then persist on `Close()`. This is the pattern used in this project:

```go
func (f *writeFile) Write(p []byte) (int, error) {
    f.written = true
    return f.buf.Write(p)
}

func (f *writeFile) Close() error {
    if !f.written {
        return nil // No data → no-op
    }
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    _, err := f.docService.Create(ctx, ...)
    return err
}
```

**Why buffer + Close()**: The WebDAV handler calls `Write()` potentially many times during a PUT. You don't want to hit the database on every chunk. Flushing on `Close()` gives you the complete content in one shot.

**Caveat**: `Close()` errors are logged by the handler's Logger but cannot change the HTTP status code — the response headers are already sent by the time the file is closed. This is a known limitation.

### Readdir Behavior

The handler calls `Readdir` for PROPFIND with Depth > 0:

```go
func (d *dirFile) Readdir(count int) ([]os.FileInfo, error) {
    if count <= 0 {
        // Return all remaining entries
        entries := d.entries[d.pos:]
        d.pos = len(d.entries)
        return entries, nil
    }
    // Return up to count entries, io.EOF at end
    if d.pos >= len(d.entries) {
        return nil, io.EOF
    }
    end := min(d.pos+count, len(d.entries))
    entries := d.entries[d.pos:end]
    d.pos = end
    if d.pos >= len(d.entries) {
        return entries, io.EOF
    }
    return entries, nil
}
```

**Key detail**: When returning the last batch with `count > 0`, return both the entries AND `io.EOF` in the same call. Some implementations incorrectly return entries with nil error, then EOF on the next call — this works but causes an extra round-trip.

## Lock System

### MemLS

`NewMemLS()` provides an in-memory lock system. Suitable for single-process deployments. Locks are lost on restart.

Internals:
- Stores locks in a map keyed by token (UUID URN)
- Maintains a path→token index for Confirm lookups
- Handles infinite-depth locks by checking path prefixes
- Thread-safe (internal mutex)

### Per-Vault Isolation

This project uses one `MemLS` per vault to prevent cross-vault lock interference:

```go
var lockSystems sync.Map // vaultID → webdav.LockSystem

getLockSystem := func(vaultID string) webdav.LockSystem {
    if ls, ok := lockSystems.Load(vaultID); ok {
        return ls.(webdav.LockSystem)
    }
    ls, _ := lockSystems.LoadOrStore(vaultID, webdav.NewMemLS())
    return ls.(webdav.LockSystem)
}
```

`LoadOrStore` handles the race where two requests for the same vault arrive simultaneously — only one `MemLS` is created and stored.

### Distributed Lock Systems

For multi-process deployments, implement `LockSystem` backed by a distributed store (Redis, database). Key requirements:
- Token uniqueness (UUID URNs are sufficient)
- Atomic Confirm (check + hold must be atomic)
- TTL expiration for lock cleanup
- Path-prefix matching for infinite-depth locks

## WebDAV Protocol Coverage

### DAV Compliance Classes

The handler advertises `DAV: 1, 2` in OPTIONS responses:
- **Class 1**: Basic WebDAV (PROPFIND, PROPPATCH, MKCOL, GET, PUT, DELETE, COPY, MOVE)
- **Class 2**: Locking support (LOCK, UNLOCK)
- **Class 3**: Not supported (would require `If` header extensions beyond what's implemented)

### Depth Header

Used by PROPFIND and COPY:
- `Depth: 0` — target resource only
- `Depth: 1` — target + immediate children
- `Depth: infinity` — target + all descendants (PROPFIND only; COPY always recurses)

The handler rejects `Depth: infinity` PROPFIND by default for performance. To allow it, the FileSystem must handle potentially large Readdir results.

### Multi-Status Responses (207)

PROPFIND, PROPPATCH, COPY, MOVE, and DELETE can return 207 Multi-Status with per-resource status. The handler generates the XML automatically from FileSystem/LockSystem return values.

## Performance

### Streaming vs Buffering

- **GET**: The handler uses `http.ServeContent` which supports range requests and `If-Modified-Since`. It calls `Seek` on the File, so `bytes.Reader` works efficiently.
- **PUT**: The handler reads the request body and writes to the File. For large files, consider implementing `io.ReaderFrom` on your write file to avoid double-buffering.
- **PROPFIND Depth 1**: Calls `Readdir(-1)` to get all children, then `Stat()` on each. For directories with thousands of entries, this can be slow. Consider caching directory metadata.

### PROPFIND Depth Infinity

Can be expensive for deep trees. The handler walks the entire subtree calling `OpenFile` + `Readdir` at each level. Mitigation:
- Return `403 Forbidden` for `Depth: infinity` requests on large trees
- Limit directory depth in your FileSystem implementation
- Use `Stat` for metadata-only queries (the handler does this for each entry)

### Concurrent Access

- The handler is safe for concurrent use
- `MemLS` is internally synchronized (mutex)
- Your `FileSystem` must handle concurrent access — the handler doesn't serialize calls
- `Confirm` + `release` pattern means locks are held for the duration of the request

### Lightweight Stat

For PROPFIND, the handler calls `Stat()` on every listed resource. Use lightweight metadata queries that don't load file content. This project uses `GetDocumentMetaByPath` (returns content length + timestamps, not content) instead of `GetDocumentByPath`.

## Security

### Authentication

**None built in.** You must implement auth in a wrapper handler or middleware. Common patterns:
- HTTP Basic Auth (this project's approach — password = API token)
- Digest Auth (more complex but doesn't send passwords in cleartext)
- Cookie/session-based (unusual for WebDAV)

Always set `WWW-Authenticate` header on 401 responses so clients prompt for credentials.

### Path Traversal

`Dir` sanitizes paths to prevent escaping the directory root. If implementing a custom `FileSystem`, ensure:
- `path.Clean()` all input paths
- Reject or normalize `..` components
- Never concatenate raw user paths with filesystem paths

This project normalizes all paths through `normalizeName()`:
```go
func normalizeName(name string) string {
    name = path.Clean(name)
    if name == "." || name == "" {
        return "/"
    }
    if !strings.HasPrefix(name, "/") {
        name = "/" + name
    }
    return name
}
```

### TLS

WebDAV over plain HTTP exposes credentials (Basic Auth) and content. Always use TLS in production. The package itself doesn't handle TLS — use `http.ListenAndServeTLS` or a reverse proxy.

### PROPFIND DoS

A `Depth: infinity` PROPFIND on a deep tree can consume significant server resources. Consider:
- Rejecting `Depth: infinity` requests
- Rate limiting PROPFIND requests
- Setting timeouts on context

### CSRF

WebDAV methods (PUT, DELETE, MKCOL, etc.) bypass browser CSRF protections since they're not standard HTML form methods. However, if your WebDAV endpoint shares cookies with a web UI, consider CSRF tokens for non-WebDAV methods. This project uses Origin/Referer checking for the web UI, while WebDAV uses separate Basic Auth.

## Client Quirks

### macOS Finder

- Sends `._*` resource fork files and `.DS_Store` on every file operation
- Expects `DAV: 1, 2` header on OPTIONS response — without it, Finder won't recognize the server as WebDAV
- Performs PROPFIND with Depth 0 on the root before mounting
- May cache aggressively — set proper `ETag` and `Last-Modified` headers
- **This project's solution**: `nopFile` accepts and silently discards `._*` and `.DS_Store` writes; reads return `os.ErrNotExist`

### Windows Mini-Redirector (Explorer)

- Has a 50MB file size limit by default (registry setting `FileSizeLimitInBytes`)
- Requires the path to end with `/` for directory browsing
- May not work with self-signed certificates
- Sends `Translate: f` header (not standard)
- Caches heavily — may need server restart to see changes

### Linux Clients

- `cadaver` and `davfs2` are well-behaved
- GNOME Files (gvfs) works but may issue many redundant PROPFINDs
- KDE Dolphin uses `kio-webdav`, generally compliant

### curl

Useful for testing:
```bash
# List directory
curl -X PROPFIND -H "Depth: 1" http://localhost:8080/dav/default/ -u :token

# Upload file
curl -T file.md http://localhost:8080/dav/default/file.md -u :token

# Download file
curl http://localhost:8080/dav/default/file.md -u :token

# Delete file
curl -X DELETE http://localhost:8080/dav/default/file.md -u :token

# Create directory
curl -X MKCOL http://localhost:8080/dav/default/newdir/ -u :token
```

## Known Issues

### In `golang.org/x/net/webdav`

1. **No built-in Depth infinity limit** — the handler doesn't cap recursion depth for PROPFIND
2. **Close() errors are swallowed** — when the handler closes a File after PUT, errors from Close() are logged but don't affect the HTTP response (response is already committed)
3. **No partial PUT (Content-Range)** — the package doesn't support partial uploads
4. **No DAV Class 3** — extended `If` header conditions aren't fully supported
5. **Dead property storage is opt-in** — without `DeadPropsHolder`, PROPPATCH returns 403 for custom properties

### General WebDAV Gotchas

- Lock tokens must survive server restarts for reliable editing (MemLS doesn't)
- COPY/MOVE may trigger multiple FileSystem calls — ensure atomicity if needed
- Some clients send `Expect: 100-continue` — Go's `net/http` handles this automatically

## Testing Patterns

### Unit Testing with httptest

```go
func TestWebDAV(t *testing.T) {
    handler := &webdav.Handler{
        FileSystem: webdav.NewMemFS(),
        LockSystem: webdav.NewMemLS(),
    }
    srv := httptest.NewServer(handler)
    defer srv.Close()

    // PUT a file
    req, _ := http.NewRequest(http.MethodPut, srv.URL+"/test.md", strings.NewReader("# Hello"))
    resp, _ := http.DefaultClient.Do(req)
    if resp.StatusCode != http.StatusCreated {
        t.Fatalf("PUT status = %d, want 201", resp.StatusCode)
    }

    // GET it back
    resp, _ = http.Get(srv.URL + "/test.md")
    body, _ := io.ReadAll(resp.Body)
    if string(body) != "# Hello" {
        t.Fatalf("GET body = %q, want %q", body, "# Hello")
    }
}
```

### Testing Custom FileSystem

Mock the FileSystem interface:

```go
type mockFS struct {
    webdav.FileSystem
    openFileFn func(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error)
}

func (m *mockFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
    return m.openFileFn(ctx, name, flag, perm)
}
```

### Litmus Test Suite

The [litmus WebDAV test suite](http://www.webdav.org/neon/litmus/) is the standard compliance test. Run it against your server:

```bash
litmus http://localhost:8080/dav/ username password
```

## Patterns from This Project

### Per-Vault WebDAV Handler

Rather than one global handler, this project creates a per-request handler with vault-scoped FileSystem and LockSystem:

```go
davFS := NewFS(vaultID, dbClient, docService, vaultSvc)
davHandler := &webdav.Handler{
    FileSystem: davFS,
    LockSystem: getLockSystem(vaultID),
    Prefix:     pathPrefix + vaultName,
}
davHandler.ServeHTTP(w, r)
```

This is slightly more allocation per request but gives clean vault isolation.

### nopFile for OS Metadata

macOS Finder sends `._*` and `.DS_Store` files. Rejecting them causes Finder to abort the entire operation. The `nopFile` pattern silently accepts writes (discarding content) and returns `io.EOF` on reads:

```go
func (f *nopFile) Write(p []byte) (int, error) { return len(p), nil }
func (f *nopFile) Read([]byte) (int, error)    { return 0, io.EOF }
```

### Markdown-Only Restriction

This project only stores markdown. Non-`.md` file writes return a custom error wrapping `os.ErrPermission`:

```go
var errNotMarkdown = fmt.Errorf("only markdown files (.md) are allowed: %w", os.ErrPermission)
```

Wrapping `os.ErrPermission` ensures the handler returns 403 (not 500).

### Write Pipeline on Close

When a file is saved via WebDAV PUT, the full document pipeline runs on `Close()`:
1. Content is buffered during `Write()` calls
2. On `Close()`, `docService.Create()` runs the pipeline: parse → embed → link → chunk
3. A fresh `context.Background()` with timeout is used because the original request context may be cancelled

### DAV Header for Client Discovery

The handler sets `DAV: 1, 2` on **every** response (not just OPTIONS) so clients like macOS Finder detect WebDAV support even on initial auth probes:

```go
w.Header().Set("DAV", "1, 2")
```

### Structured Error Logging

The Logger callback filters by error type — `os.ErrNotExist` and `os.ErrPermission` are debug-level (expected in normal operation), while other errors are warnings:

```go
Logger: func(r *http.Request, err error) {
    if err == nil {
        return
    }
    if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
        slog.Debug("webdav request", "method", r.Method, "path", r.URL.Path, "error", err)
    } else {
        slog.Warn("webdav request failed", "method", r.Method, "path", r.URL.Path, "error", err)
    }
},
```
