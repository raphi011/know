# Web UI v1 — Reference Notes

## CodeMirror 6 Setup

**Core packages:**
- `codemirror` ^6.0.1 — core
- `@codemirror/lang-markdown` ^6.3.0 — markdown syntax
- `@codemirror/theme-one-dark` ^6.1.0 — dark theme
- `@codemirror/view` ^6.36.0 — editor view
- `@codemirror/state` ^6.5.0 — state management

**Extension stack:**

```typescript
import { markdown } from '@codemirror/lang-markdown'
import { oneDark } from '@codemirror/theme-one-dark'
import { EditorView, keymap } from '@codemirror/view'
import { history, defaultKeymap, historyKeymap } from 'codemirror'

const extensions = [
  markdown(),
  oneDark,
  EditorView.lineWrapping,
  history(),
  keymap.of([...defaultKeymap, ...historyKeymap]),
  // Custom save shortcut
  keymap.of([{ key: "Mod-s", run: () => { onSave(); return true } }]),
  // Change listener
  EditorView.updateListener.of(update => {
    if (update.docChanged) onChange(update.state.doc.toString())
  }),
]
```

**Gotchas:**
- Line wrapping is `EditorView.lineWrapping` (a static extension), not a config option
- To replace document content (e.g. switching files), dispatch a full replacement transaction — don't recreate the editor
- Destroy editor view on component unmount to avoid memory leaks
- Theme inherits from parent CSS — set `.cm-editor { height: 100% }` and `.cm-scroller { overflow: auto }` for fill behavior

## Go Server Integration

- Built web assets embedded via `//go:embed all:dist` in `web/embed.go`
- Single binary deployment, no separate asset server
- SPA fallback: non-matching paths serve `index.html`
- Routes: `/` (SPA), `/query` (GraphQL), `/playground`, `/health`

## GraphQL Queries (Document CRUD)

```graphql
query ListDocuments($labels: [String!]) {
  entities(type: "document", labels: $labels, limit: 500) { id, name, updatedAt, labels }
}
query GetEntity($id: ID!) {
  entity(id: $id) { id, name, content, labels, updatedAt }
}
mutation UpdateEntityContent($id: ID!, $content: String!) {
  updateEntityContent(id: $id, content: $content) { id, updatedAt }
}
mutation UpdateEntityLabels($id: ID!, $input: EntityUpdate!) {
  updateEntity(id: $id, input: $input) { id, labels }
}
query ListLabels { labels { label, count } }
subscription ChatStream($conversationId: ID!, $message: String!, $history: [ChatMessageInput!]!, $input: SearchInput) {
  chatStream(...) { token, done, error }
}
```

## Dev Setup

- Vite proxy: `/query` → `http://localhost:8484` with `ws: true` for subscriptions
- `just web-dev` for Vite dev server on :5173
- `just web-build` → `web/dist/` → embedded in Go binary via `just build-server`
