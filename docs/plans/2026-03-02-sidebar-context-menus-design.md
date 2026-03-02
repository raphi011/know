# Sidebar Context Menus

**Date:** 2026-03-02

## Overview

Add right-click context menus to the document tree sidebar, enabling file management operations: create documents, create folders, rename, and delete.

## Decisions

### Context Menu Component

New `ContextMenu` primitive in `components/ui/context-menu.tsx` wrapping Headless UI `Menu`, positioned at cursor coordinates via an invisible anchor element. Reuses existing `DropdownMenuItem` styling and gets accessibility (focus trap, arrow keys, Escape) for free.

### Context-Sensitive Menus

| Target | Actions |
|---|---|
| Folder | New Document, New Folder, Rename, Delete |
| Document | Rename, Delete |
| Empty area (below tree) | New Document, New Folder |

### Inline Editing

All create/rename operations use inline text inputs directly in the tree (VS Code style):
- **New Document / New Folder**: Temporary node appears at target location with focused input. Enter confirms, Escape cancels.
- **Rename**: Node name transforms into pre-filled editable input with text selected.
- **Validation**: No empty names, no `/` in names, no duplicate siblings. Red border on invalid.

### Virtual Folders

Folders are derived from document paths. "New Folder" is optimistic — the folder appears in the tree immediately but only persists once a document is created inside it.

### Backend Mutations

New GraphQL mutations for atomic folder operations:
- `deleteDocumentsByPrefix(vaultId, pathPrefix)` — deletes all docs under a folder
- `moveDocumentsByPrefix(vaultId, oldPrefix, newPrefix)` — renames/moves all docs under a folder

Individual document operations use existing `createDocument`, `deleteDocument`, `moveDocument`.

### Data Flow

1. Right-click on tree node → store `{x, y, targetNode}` in state
2. Render `ContextMenu` at position via portal
3. User selects action → inline input or confirm dialog
4. Action calls server action in `mutations.ts` → GraphQL mutation → Go backend
5. `router.refresh()` re-fetches document list → tree rebuilds

### VaultId Threading

`AppShellWrapper` passes vault ID through `DocSidebar` → `DocTree` for mutation calls.

### New Server Actions (`mutations.ts`)

```ts
createDocument(vaultId, path, content): ActionResult
deleteDocument(vaultId, path): ActionResult
moveDocument(vaultId, oldPath, newPath): ActionResult
deleteDocumentsByPrefix(vaultId, pathPrefix): ActionResult
moveDocumentsByPrefix(vaultId, oldPrefix, newPrefix): ActionResult
```

### i18n

New `"tree"` namespace in `messages/{locale}.json`:
- `newDocument`, `newFolder`, `rename`, `delete`
- `deleteConfirmTitle`, `deleteConfirmDescription`, `deleteFolderConfirmDescription`
- `nameRequired`, `nameInvalid`, `nameDuplicate`

## Files Affected

### New Files
- `web/components/ui/context-menu.tsx` — ContextMenu primitive
- `web/components/inline-tree-input.tsx` — Inline name input for tree

### Modified Files (Frontend)
- `web/components/doc-tree.tsx` — Add context menu handlers, inline editing state
- `web/components/domain/doc-sidebar.tsx` — Pass vaultId through
- `web/app/(main)/app-shell-wrapper.tsx` — Pass vaultId to DocSidebar
- `web/app/lib/knowhow/mutations.ts` — New mutation functions
- `web/messages/en.json` — Add tree namespace
- `web/messages/de.json` — Add tree namespace (German)

### Modified Files (Backend)
- `internal/graph/schema.graphqls` — Add folder mutation types
- `internal/graph/schema.resolvers.go` — Implement folder resolvers
- `internal/document/service.go` (or similar) — Folder-level delete/move logic
- `internal/db/` — Folder-level DB queries
