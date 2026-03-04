# Sidebar Drag and Drop

**Date:** 2026-03-04
**Status:** Approved

## Summary

Add drag-and-drop to the knowhow sidebar:
1. **Internal DnD** — drag documents/folders to move them between folders
2. **External file drop** — drag `.md` files from the OS into the sidebar to import them

## Decisions

- **Library:** `@dnd-kit/core` — modern, hooks-based, accessible, React 19 compatible
- **File types:** Markdown only (`.md`)
- **Move behavior:** Move into folders only, alphabetical sort preserved (no custom ordering)
- **Drop targeting:** User targets specific folders (not always-root)
- **Folders are virtual** — moves use `moveDocument` / `moveDocumentsByPrefix` mutations

## Component Architecture

```
DndContext (dnd-kit)
├── DocTree
│   └── TreeNodeItem (recursive)
│       ├── useDraggable()     ← all items draggable
│       └── useDroppable()     ← folders accept drops
├── DragOverlay              ← floating preview while dragging
└── FileDropZone             ← HTML5 file drop overlay for OS files
```

- `TreeNodeItem` gets both `useDraggable` + `useDroppable` (folders only get droppable)
- Root drop zone = empty area at bottom of tree
- `FileDropZone` is separate from dnd-kit (uses native HTML5 file API)
- `DragOverlay` renders ghost preview of dragged item

## Internal Drag (Sidebar Reordering)

### Drop Rules

| Drop onto | Result |
|-----------|--------|
| Folder | Move item inside that folder |
| Root area | Move item to root level |
| Same parent | No-op |
| Self or own descendant | Rejected |

### Backend Calls

- Single document: `moveDocument(vaultId, oldPath, newPath)`
- Folder: `moveDocumentsByPrefix(vaultId, oldPrefix, newPrefix)`

### Optimistic Updates

- Immediately update tree UI
- Revert + error toast on mutation failure
- Client-side conflict check before mutation (same-name in target folder)

### Auto-expand

- Folders auto-expand after ~500ms hover while dragging

## External File Drop (OS to Sidebar)

### Flow

1. User drags `.md` file(s) from OS into browser
2. `dragenter` activates `FileDropZone` overlay on sidebar
3. Folders highlight as drop targets (same as internal)
4. On `drop`:
   - Read file content via `FileReader`
   - Validate `.md` extension
   - Determine path: `<folder-path>/<filename>` or `<filename>`
   - Conflict check (name collision)
   - Call `createDocument(vaultId, path, content)`

### Multi-file

- Accept multiple `.md` files per drop
- Create sequentially (avoid race conditions)
- Progress feedback: "Importing 3 of 5 files..."

### Edge Cases

- Non-`.md` files: skip, toast "Skipped N non-markdown files"
- Name collision: skip file, toast "filename.md already exists"
- Empty file: create with empty content
- Large files: no limit (markdown is typically small)

## Visual Feedback

### Internal Drag

- Source item: 50% opacity
- `DragOverlay`: floating card with icon + name
- Valid targets: blue highlight border/background
- Invalid targets: no highlight
- Root zone: "Move to root" indicator

### External File Drag

- Sidebar overlay: dashed border + "Drop .md files here"
- Same folder highlighting as internal
- Non-`.md` detected: warning-style overlay

### After Drop

- Success: immediate tree update, brief green flash on item
- Error: red toast with message
- External import: "Imported N files" toast

## Accessibility

- dnd-kit keyboard DnD: Space to grab, Arrow keys to navigate, Space to drop
- Screen reader announcements via dnd-kit `announcements` config
- Focus moves to dropped item's new location after drop
