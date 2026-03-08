# Go Templ

[Templ](https://templ.guide/) is a type-safe HTML templating language for Go. It compiles `.templ` files to Go code, giving compile-time type checking for all template data.

**Version**: v0.3.1001 (via `go tool templ`)

## File Organization

```
internal/web/templates/
├── layout.templ           # Base HTML shell (<html>, <head>, <body>)
├── pages/                 # Full page components (login, doc_view, settings, agent)
├── components/            # Reusable UI components (sidebar, search dialog)
└── partials/              # HTMX partial responses (search_results, doc_content, version_list)
```

**Package per folder**: each directory is its own Go package (`templates`, `pages`, `components`, `partials`).

### Pages vs Partials

- **Pages** return full HTML (wrapped in `Layout`) — served on initial page load
- **Partials** return HTML fragments — served by `/hx/*` endpoints for HTMX swaps

## Component Patterns

### Struct-Based Props

Every component takes a single data struct:

```go
type DocViewData struct {
    Title   string
    Locale  string
    Theme   string
    DocID   string
    Content string
    Sidebar components.SidebarData
    T       func(string) string
}

templ DocViewPage(data DocViewData) {
    @templates.Layout(templates.LayoutData{
        Title:  data.Title,
        Locale: data.Locale,
        Theme:  data.Theme,
        T:      data.T,
    }) {
        // page content
    }
}
```

Always include `Locale`, `Theme`, and `T` (i18n function) in page-level structs.

### Children Composition

Use `{ children... }` to accept nested content:

```go
templ Layout(data LayoutData) {
    <html>
        <body>
            { children... }
        </body>
    </html>
}

// Usage:
@Layout(layoutData) {
    <h1>Hello</h1>
}
```

### Calling Other Components

```go
@components.Sidebar(data.Sidebar)
@partials.DocContent(data.Content, data.VaultID, data.Path)
```

### Recursive Components

Templ components can call themselves (e.g., folder trees):

```go
templ FolderTree(vaultID string, folders []Folder) {
    for _, f := range folders {
        <li>
            <a href={ templ.SafeURL("/docs" + f.Path) }>{ f.Name }</a>
            if len(f.Children) > 0 {
                <ul>
                    @FolderTree(vaultID, f.Children)
                </ul>
            }
        </li>
    }
}
```

## Conditional Rendering

### Go `if` statements

```go
if data.Error != "" {
    <div class="text-red-500">{ data.Error }</div>
}
```

### Conditional CSS Classes with `templ.KV`

```go
<html class={ templ.KV("dark", data.Theme == "dark") }>
```

`templ.KV(className, condition)` adds the class only when the condition is true.

### Conditional Attributes (select options)

```go
for _, v := range data.Vaults {
    if v.Selected {
        <option value={ v.ID } selected>{ v.Name }</option>
    } else {
        <option value={ v.ID }>{ v.Name }</option>
    }
}
```

## URL Handling

```go
// Safe URL encoding (prevents XSS in href attributes)
href={ templ.SafeURL("/docs" + folder.Path) }

// Query parameter encoding
sse-connect={ "/hx/doc/events?vault=" + url.QueryEscape(vaultID) + "&path=" + url.QueryEscape(path) }

// Dynamic attribute values with fmt
hx-get={ fmt.Sprintf("/hx/versions?doc=%s", data.DocID) }
```

## Raw HTML

For pre-rendered content (markdown, trusted HTML):

```go
@templ.Raw(renderedMarkdown)
```

Only use for content you control (e.g., server-rendered markdown). Never use with user-supplied HTML.

## Generation

```bash
just templ-generate    # Runs: go tool templ generate ./internal/web/templates/...
just generate-all      # Runs: gqlgen generate + templ generate + tailwind build
```

Always run after editing `.templ` files. Generated `*_templ.go` files are committed to the repo.

## Rendering in Handlers

### Full Pages

```go
component := pages.DocViewPage(pages.DocViewData{...})
w.Header().Set("Content-Type", "text/html; charset=utf-8")
if err := component.Render(r.Context(), w); err != nil {
    slog.Error("render failed", "error", err)
    http.Error(w, "internal error", http.StatusInternalServerError)
}
```

### HTMX Partials (Buffered)

```go
func renderBuffered(w http.ResponseWriter, r *http.Request, component renderComponent) {
    var buf bytes.Buffer
    if err := component.Render(r.Context(), &buf); err != nil {
        slog.Error("render failed", "error", err)
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    buf.WriteTo(w)
}
```

**Why buffer?** If rendering fails mid-write, the client receives partial/corrupt HTML. Buffering ensures either complete HTML or an error response — never a mix.

## i18n Pattern

Translation function passed as a prop through the component tree:

```go
// Handler
t := T(locale) // Returns func(string) string for the given locale

// Template
<h1>{ data.T("login.title") }</h1>
<button>{ data.T("action.save") }</button>
```

Translation files: `internal/web/messages/en.json`, `internal/web/messages/de.json`

## Helper Functions

Small helpers can live in `.templ` files:

```go
func intStr(n int) string {
    return fmt.Sprint(n)
}
```

For shared helpers, put them in a separate `.go` file in the same package.

## Best Practices

1. **One struct per component** — avoids parameter sprawl, makes dependencies explicit
2. **Pages wrap in Layout** — every page component calls `@templates.Layout(...) { ... }`
3. **Partials never wrap in Layout** — they return fragments only
4. **Use `templ.SafeURL()`** for all dynamic `href` values
5. **Use `templ.Raw()` sparingly** — only for server-rendered trusted content
6. **Buffered rendering for partials** — prevents corrupt HTML on error
7. **External JS only** — no inline `<script>` blocks in templ files; load JS from `/static/js/`
8. **Import packages in templ** — `fmt`, `url`, etc. can be imported at the top of `.templ` files
