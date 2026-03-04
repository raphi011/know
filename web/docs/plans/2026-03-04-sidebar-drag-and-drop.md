# Sidebar Drag and Drop Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add drag-and-drop to the knowhow sidebar for moving documents/folders between folders and importing `.md` files from the OS.

**Architecture:** Wrap the existing `DocTree` component in dnd-kit's `DndContext`. Each `TreeNodeItem` becomes draggable; folders become drop targets. A separate `FileDropZone` handles native HTML5 file drops from the OS. Toast notifications provide feedback.

**Tech Stack:** @dnd-kit/core, React 19, Next.js 16, TypeScript, Vitest

**Design doc:** `docs/plans/2026-03-04-sidebar-drag-and-drop-design.md`

---

### Task 1: Install @dnd-kit dependencies

**Files:**
- Modify: `package.json`

**Step 1: Install packages**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun add @dnd-kit/core @dnd-kit/utilities`

**Step 2: Verify installation**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run build`
Expected: Build succeeds

**Step 3: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add package.json bun.lock
git commit -m "chore: add @dnd-kit/core and @dnd-kit/utilities"
```

---

### Task 2: Create toast context (useToast hook)

The existing `Toast` component (`components/ui/toast.tsx`) is a presentational component with no state management. We need a context + hook so any component can fire toasts.

**Files:**
- Create: `components/ui/toast-provider.tsx`
- Test: `components/ui/toast-provider.test.tsx`

**Step 1: Write the failing test**

Create `components/ui/toast-provider.test.tsx`:

```tsx
import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { ToastProvider, useToast } from "./toast-provider";
import type { ReactNode } from "react";

const wrapper = ({ children }: { children: ReactNode }) => (
  <ToastProvider>{children}</ToastProvider>
);

describe("useToast", () => {
  it("starts with no toasts", () => {
    const { result } = renderHook(() => useToast(), { wrapper });
    expect(result.current.toasts).toEqual([]);
  });

  it("adds a toast", () => {
    const { result } = renderHook(() => useToast(), { wrapper });
    act(() => {
      result.current.toast({ variant: "success", title: "Done" });
    });
    expect(result.current.toasts).toHaveLength(1);
    expect(result.current.toasts[0].title).toBe("Done");
  });

  it("dismisses a toast", () => {
    const { result } = renderHook(() => useToast(), { wrapper });
    act(() => {
      result.current.toast({ variant: "error", title: "Oops" });
    });
    const id = result.current.toasts[0].id;
    act(() => {
      result.current.dismiss(id);
    });
    expect(result.current.toasts).toEqual([]);
  });

  it("auto-dismisses after timeout", async () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useToast(), { wrapper });
    act(() => {
      result.current.toast({ variant: "info", title: "Temp", duration: 1000 });
    });
    expect(result.current.toasts).toHaveLength(1);
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current.toasts).toEqual([]);
    vi.useRealTimers();
  });
});
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run vitest --project unit --run components/ui/toast-provider.test.tsx`
Expected: FAIL — module not found

**Step 3: Install test dependency if needed**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun add -d @testing-library/react`

**Step 4: Write the implementation**

Create `components/ui/toast-provider.tsx`:

```tsx
"use client";

import {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Toast } from "./toast";

type ToastVariant = "success" | "error" | "info";

interface ToastItem {
  id: string;
  variant: ToastVariant;
  title: string;
  description?: string;
  duration?: number;
}

interface ToastInput {
  variant: ToastVariant;
  title: string;
  description?: string;
  duration?: number;
}

interface ToastContextValue {
  toasts: ToastItem[];
  toast: (input: ToastInput) => void;
  dismiss: (id: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

let nextId = 0;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const timers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const dismiss = useCallback((id: string) => {
    const timer = timers.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timers.current.delete(id);
    }
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback(
    (input: ToastInput) => {
      const id = String(++nextId);
      const duration = input.duration ?? 4000;
      setToasts((prev) => [...prev, { ...input, id }]);
      const timer = setTimeout(() => dismiss(id), duration);
      timers.current.set(id, timer);
    },
    [dismiss],
  );

  return (
    <ToastContext value={{ toasts, toast, dismiss }}>
      {children}
      <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
        {toasts.map((t) => (
          <Toast
            key={t.id}
            variant={t.variant}
            title={t.title}
            description={t.description}
            open={true}
            onClose={() => dismiss(t.id)}
          />
        ))}
      </div>
    </ToastContext>
  );
}

export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}
```

**Step 5: Run tests to verify they pass**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run vitest --project unit --run components/ui/toast-provider.test.tsx`
Expected: All 4 tests PASS

**Step 6: Add ToastProvider to root layout**

Modify `app/(main)/app-shell-wrapper.tsx`. Wrap the outermost element with `<ToastProvider>`:

```tsx
import { ToastProvider } from "@/components/ui/toast-provider";

// In render, wrap everything:
return (
  <ToastProvider>
    {/* existing content */}
  </ToastProvider>
);
```

**Step 7: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add components/ui/toast-provider.tsx components/ui/toast-provider.test.tsx app/\(main\)/app-shell-wrapper.tsx
git commit -m "feat: add toast context with useToast hook"
```

---

### Task 3: Create DnD utility functions with tests

Pure functions for drop validation, path resolution, and file filtering. TDD.

**Files:**
- Create: `app/lib/knowhow/dnd-utils.ts`
- Test: `app/lib/knowhow/dnd-utils.test.ts`

**Step 1: Write the failing tests**

Create `app/lib/knowhow/dnd-utils.test.ts`:

```ts
import { describe, it, expect } from "vitest";
import {
  resolveDropPath,
  hasNameConflict,
  isDescendantPath,
  validateInternalDrop,
  filterMarkdownFiles,
} from "./dnd-utils";
import type { DocumentSummary, TreeNode } from "./types";

describe("resolveDropPath", () => {
  it("moves doc to root", () => {
    expect(resolveDropPath("folder/doc.md", "")).toBe("doc.md");
  });
  it("moves doc into folder", () => {
    expect(resolveDropPath("doc.md", "folder")).toBe("folder/doc.md");
  });
  it("moves doc between folders", () => {
    expect(resolveDropPath("a/doc.md", "b")).toBe("b/doc.md");
  });
  it("moves folder to root", () => {
    expect(resolveDropPath("parent/child", "")).toBe("child");
  });
  it("moves folder into folder", () => {
    expect(resolveDropPath("child", "parent")).toBe("parent/child");
  });
});

describe("hasNameConflict", () => {
  const docs: DocumentSummary[] = [
    { id: "1", vaultId: "v", path: "readme.md", title: "", labels: [], docType: null, createdAt: "", updatedAt: "" },
    { id: "2", vaultId: "v", path: "folder/notes.md", title: "", labels: [], docType: null, createdAt: "", updatedAt: "" },
  ];

  it("detects conflict at root", () => {
    expect(hasNameConflict(docs, "readme.md")).toBe(true);
  });
  it("detects conflict in folder", () => {
    expect(hasNameConflict(docs, "folder/notes.md")).toBe(true);
  });
  it("no conflict for new path", () => {
    expect(hasNameConflict(docs, "folder/new.md")).toBe(false);
  });
  it("case-insensitive", () => {
    expect(hasNameConflict(docs, "README.md")).toBe(true);
  });
});

describe("isDescendantPath", () => {
  it("direct child is descendant", () => {
    expect(isDescendantPath("parent", "parent/child")).toBe(true);
  });
  it("nested descendant", () => {
    expect(isDescendantPath("a", "a/b/c")).toBe(true);
  });
  it("not a descendant", () => {
    expect(isDescendantPath("a", "b/c")).toBe(false);
  });
  it("same path is descendant", () => {
    expect(isDescendantPath("a", "a")).toBe(true);
  });
  it("partial name match is not descendant", () => {
    expect(isDescendantPath("app", "application/file.md")).toBe(false);
  });
});

describe("validateInternalDrop", () => {
  it("rejects drop on self", () => {
    expect(validateInternalDrop("a", "a").valid).toBe(false);
  });
  it("rejects folder drop on own descendant", () => {
    expect(validateInternalDrop("a", "a/b").valid).toBe(false);
  });
  it("rejects drop on same parent (doc already there)", () => {
    expect(validateInternalDrop("folder/doc.md", "folder").valid).toBe(false);
  });
  it("allows doc move to different folder", () => {
    expect(validateInternalDrop("a/doc.md", "b").valid).toBe(true);
  });
  it("allows doc move to root", () => {
    expect(validateInternalDrop("folder/doc.md", "").valid).toBe(true);
  });
  it("allows folder move to different folder", () => {
    expect(validateInternalDrop("a", "b").valid).toBe(true);
  });
});

describe("filterMarkdownFiles", () => {
  const makeFile = (name: string) => new File(["content"], name);

  it("accepts .md files", () => {
    const files = [makeFile("readme.md"), makeFile("notes.md")];
    const result = filterMarkdownFiles(files);
    expect(result.valid).toHaveLength(2);
    expect(result.skipped).toBe(0);
  });

  it("rejects non-.md files", () => {
    const files = [makeFile("image.png"), makeFile("readme.md")];
    const result = filterMarkdownFiles(files);
    expect(result.valid).toHaveLength(1);
    expect(result.valid[0].name).toBe("readme.md");
    expect(result.skipped).toBe(1);
  });

  it("handles empty list", () => {
    const result = filterMarkdownFiles([]);
    expect(result.valid).toHaveLength(0);
    expect(result.skipped).toBe(0);
  });
});
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run vitest --project unit --run app/lib/knowhow/dnd-utils.test.ts`
Expected: FAIL — module not found

**Step 3: Implement the utility functions**

Create `app/lib/knowhow/dnd-utils.ts`:

```ts
import type { DocumentSummary } from "./types";

/** Get the basename (last segment) of a path */
function basename(path: string): string {
  const parts = path.split("/");
  return parts[parts.length - 1];
}

/** Get the parent folder path, or "" for root-level items */
function parentPath(path: string): string {
  const idx = path.lastIndexOf("/");
  return idx === -1 ? "" : path.slice(0, idx);
}

/** Compute the new path when moving an item to a target folder */
export function resolveDropPath(
  draggedPath: string,
  targetFolderPath: string,
): string {
  const name = basename(draggedPath);
  return targetFolderPath ? `${targetFolderPath}/${name}` : name;
}

/** Check if a document already exists at the given path (case-insensitive) */
export function hasNameConflict(
  documents: DocumentSummary[],
  newPath: string,
): boolean {
  const lower = newPath.toLowerCase();
  return documents.some((d) => d.path.toLowerCase() === lower);
}

/** Check if childPath is equal to or nested under parentPathStr */
export function isDescendantPath(
  parentPathStr: string,
  childPath: string,
): boolean {
  if (childPath === parentPathStr) return true;
  return childPath.startsWith(parentPathStr + "/");
}

/** Validate whether an internal drag-drop is allowed */
export function validateInternalDrop(
  draggedPath: string,
  targetFolderPath: string,
): { valid: boolean; reason?: string } {
  // Can't drop on self
  if (draggedPath === targetFolderPath) {
    return { valid: false, reason: "Cannot drop on itself" };
  }
  // Can't drop folder into its own descendant
  if (isDescendantPath(draggedPath, targetFolderPath)) {
    return { valid: false, reason: "Cannot drop into own subfolder" };
  }
  // Already in this folder (same parent)
  if (parentPath(draggedPath) === targetFolderPath) {
    return { valid: false, reason: "Already in this folder" };
  }
  return { valid: true };
}

/** Filter a list of files to only .md files */
export function filterMarkdownFiles(files: File[]): {
  valid: File[];
  skipped: number;
} {
  const valid: File[] = [];
  let skipped = 0;
  for (const file of files) {
    if (file.name.toLowerCase().endsWith(".md")) {
      valid.push(file);
    } else {
      skipped++;
    }
  }
  return { valid, skipped };
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run vitest --project unit --run app/lib/knowhow/dnd-utils.test.ts`
Expected: All tests PASS

**Step 5: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add app/lib/knowhow/dnd-utils.ts app/lib/knowhow/dnd-utils.test.ts
git commit -m "feat: add DnD utility functions with tests"
```

---

### Task 4: Wrap DocTree in DndContext with sensors

Add the dnd-kit context wrapper to the tree. No visual changes yet — just the plumbing.

**Files:**
- Modify: `components/doc-tree.tsx` (the `DocTree` component wraps around line 86-525)

**Step 1: Add DndContext imports and sensors**

At the top of `components/doc-tree.tsx`, add:

```ts
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  KeyboardSensor,
  useSensor,
  useSensors,
  type DragStartEvent,
  type DragEndEvent,
  type DragOverEvent,
} from "@dnd-kit/core";
```

**Step 2: Add sensor setup inside DocTree component**

Inside the `DocTree` function body (before the return), add:

```ts
const [activeId, setActiveId] = useState<string | null>(null);

const pointerSensor = useSensor(PointerSensor, {
  activationConstraint: { distance: 8 }, // 8px drag threshold to avoid accidental drags
});
const keyboardSensor = useSensor(KeyboardSensor);
const sensors = useSensors(pointerSensor, keyboardSensor);

function handleDragStart(event: DragStartEvent) {
  setActiveId(String(event.active.id));
}

function handleDragEnd(event: DragEndEvent) {
  setActiveId(null);
  // Will be implemented in Task 8
}
```

**Step 3: Wrap tree content in DndContext**

In the `DocTree` return, wrap the tree `<ul>` and root area with:

```tsx
<DndContext
  sensors={sensors}
  onDragStart={handleDragStart}
  onDragEnd={handleDragEnd}
>
  {/* existing <ul> tree content */}
  <DragOverlay>
    {activeId ? <div className="rounded bg-white px-2 py-1 shadow-md text-sm dark:bg-zinc-800">{activeId}</div> : null}
  </DragOverlay>
</DndContext>
```

**Step 4: Verify build**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run build`
Expected: Build succeeds

**Step 5: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add components/doc-tree.tsx
git commit -m "feat: wrap DocTree in DndContext with pointer and keyboard sensors"
```

---

### Task 5: Make TreeNodeItem draggable

Add `useDraggable` to each `TreeNodeItem` so items can be picked up.

**Files:**
- Modify: `components/doc-tree.tsx` — the `TreeNodeItem` function (nested around lines 391-521)

**Step 1: Add useDraggable import**

Add to existing dnd-kit import:

```ts
import { useDraggable } from "@dnd-kit/core";
```

**Step 2: Add useDraggable to TreeNodeItem**

Inside the `TreeNodeItem` function body, add at the top:

```ts
const { attributes, listeners, setNodeRef: setDragRef, isDragging } = useDraggable({
  id: node.path,
  data: { node },
});
```

**Step 3: Apply to the DOM element**

On the outer `<li>` or the clickable row `<div>` of each TreeNodeItem, spread the drag props:

```tsx
<div
  ref={setDragRef}
  {...listeners}
  {...attributes}
  className={cn(
    existingClasses,
    isDragging && "opacity-50",
  )}
>
  {/* existing content */}
</div>
```

**Step 4: Update DragOverlay to show proper content**

In DocTree, find the active node from the tree and render it in the overlay:

```tsx
const activeNode = activeId ? findNodeByPath(tree, activeId) : null;

// In DragOverlay:
<DragOverlay>
  {activeNode ? (
    <div className="flex items-center gap-2 rounded bg-white px-3 py-1.5 shadow-lg text-sm dark:bg-zinc-800">
      {activeNode.type === "folder" ? (
        <FolderIcon className="h-4 w-4 text-zinc-400" />
      ) : (
        <DocumentIcon className="h-4 w-4 text-zinc-400" />
      )}
      {activeNode.name}
    </div>
  ) : null}
</DragOverlay>
```

Add a helper function (can go in `dnd-utils.ts` or inline):

```ts
function findNodeByPath(nodes: TreeNode[], path: string): TreeNode | null {
  for (const node of nodes) {
    if (node.path === path) return node;
    if (node.type === "folder") {
      const found = findNodeByPath(node.children, path);
      if (found) return found;
    }
  }
  return null;
}
```

**Step 5: Verify in browser**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run dev`
Expected: Items can be picked up, ghost overlay follows cursor, source becomes semi-transparent

**Step 6: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add components/doc-tree.tsx
git commit -m "feat: make tree items draggable with drag overlay"
```

---

### Task 6: Make folders droppable + root drop zone

Add `useDroppable` to folder nodes and a root-level drop zone.

**Files:**
- Modify: `components/doc-tree.tsx`

**Step 1: Add useDroppable import**

Add to existing dnd-kit import:

```ts
import { useDroppable } from "@dnd-kit/core";
```

**Step 2: Add useDroppable to folder TreeNodeItems**

Inside `TreeNodeItem`, when `node.type === "folder"`, add:

```ts
const { setNodeRef: setDropRef, isOver } = useDroppable({
  id: `drop:${node.path}`,
  data: { folderPath: node.path },
});
```

Combine refs if the same element is both draggable and droppable. Use a callback ref:

```ts
const combinedRef = useCallback(
  (el: HTMLElement | null) => {
    setDragRef(el);
    if (node.type === "folder") setDropRef(el);
  },
  [setDragRef, setDropRef],
);
```

Apply `isOver` styling to folders:

```tsx
className={cn(
  existingClasses,
  isDragging && "opacity-50",
  isOver && "bg-blue-50 ring-1 ring-blue-300 dark:bg-blue-950 dark:ring-blue-700",
)}
```

**Step 3: Add root drop zone**

Below the tree `<ul>`, add a droppable root zone:

```tsx
function RootDropZone() {
  const { setNodeRef, isOver } = useDroppable({
    id: "drop:root",
    data: { folderPath: "" },
  });

  return (
    <div
      ref={setNodeRef}
      className={cn(
        "min-h-8 flex-1",
        isOver && "bg-blue-50 dark:bg-blue-950",
      )}
    />
  );
}
```

Add `<RootDropZone />` after the tree `<ul>` inside the `DndContext`.

**Step 4: Verify in browser**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run dev`
Expected: Folders highlight blue when dragging over them. Root area highlights when dragging to bottom.

**Step 5: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add components/doc-tree.tsx
git commit -m "feat: make folders and root zone droppable with highlight"
```

---

### Task 7: Auto-expand folders on drag hover

Folders should auto-expand after 500ms of hovering during a drag.

**Files:**
- Modify: `components/doc-tree.tsx`

**Step 1: Add hover timer logic**

In `DocTree`, add a `DragOverEvent` handler:

```ts
const expandTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

function handleDragOver(event: DragOverEvent) {
  const overId = event.over?.id;
  // Clear timer if we left the folder
  if (expandTimer.current) {
    clearTimeout(expandTimer.current);
    expandTimer.current = null;
  }
  if (!overId || typeof overId !== "string" || !overId.startsWith("drop:")) return;
  const folderPath = overId.slice(5); // Remove "drop:" prefix
  if (!folderPath) return; // Root zone, nothing to expand

  // Auto-expand after 500ms
  if (!expandedFolders.has(folderPath)) {
    expandTimer.current = setTimeout(() => {
      setExpandedFolders((prev) => new Set([...prev, folderPath]));
    }, 500);
  }
}
```

**Step 2: Wire up onDragOver**

Add `onDragOver={handleDragOver}` to the `<DndContext>`.

**Step 3: Clean up timer on drag end**

In `handleDragEnd` and `handleDragCancel`, clear the timer:

```ts
if (expandTimer.current) {
  clearTimeout(expandTimer.current);
  expandTimer.current = null;
}
```

**Step 4: Verify in browser**

Expected: Hover over collapsed folder while dragging → it expands after 500ms

**Step 5: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add components/doc-tree.tsx
git commit -m "feat: auto-expand folders on drag hover after 500ms"
```

---

### Task 8: Implement onDragEnd handler for internal moves

Wire up the drop handler to actually move documents/folders via mutations.

**Files:**
- Modify: `components/doc-tree.tsx`

**Step 1: Update DocTree props**

The `DocTree` component needs access to `documents` (flat list) for conflict detection and `vaultId` for mutations. Check current props — `vaultId` is already passed. Add `documents: DocumentSummary[]` to props if not present, and pass it from `doc-sidebar.tsx` / `app-shell-wrapper.tsx`.

**Step 2: Implement handleDragEnd**

```ts
import { resolveDropPath, hasNameConflict, validateInternalDrop } from "@/app/lib/knowhow/dnd-utils";
import { moveDocument, moveDocumentsByPrefix } from "@/app/lib/knowhow/mutations";
import { useToast } from "@/components/ui/toast-provider";

// Inside DocTree:
const { toast } = useToast();

async function handleDragEnd(event: DragEndEvent) {
  setActiveId(null);
  if (expandTimer.current) {
    clearTimeout(expandTimer.current);
    expandTimer.current = null;
  }

  const { active, over } = event;
  if (!over) return;

  const draggedPath = String(active.id);
  const overId = String(over.id);
  if (!overId.startsWith("drop:")) return;

  const targetFolderPath = overId.slice(5); // Remove "drop:" prefix

  // Validate
  const validation = validateInternalDrop(draggedPath, targetFolderPath);
  if (!validation.valid) return; // Silent no-op for invalid drops

  const newPath = resolveDropPath(draggedPath, targetFolderPath);

  // Conflict check
  if (hasNameConflict(documents, newPath)) {
    toast({
      variant: "error",
      title: `"${draggedPath.split("/").pop()}" already exists in ${targetFolderPath || "root"}`,
    });
    return;
  }

  // Determine if it's a folder or document
  const draggedNode = findNodeByPath(tree, draggedPath);
  if (!draggedNode) return;

  let result;
  if (draggedNode.type === "folder") {
    result = await moveDocumentsByPrefix(vaultId, draggedPath, newPath);
  } else {
    result = await moveDocument(vaultId, draggedPath, newPath);
  }

  if (!result.success) {
    toast({ variant: "error", title: `Failed to move: ${result.error}` });
    return;
  }

  // Refresh the page to get updated data from server
  // (router.refresh() triggers server component re-render)
  router.refresh();
}
```

**Step 3: Add router import**

```ts
import { useRouter } from "next/navigation";
// Inside DocTree:
const router = useRouter();
```

**Step 4: Verify in browser**

Test cases:
1. Drag a doc onto a folder → doc moves into folder
2. Drag a doc to root zone → doc moves to root
3. Drag a folder onto another folder → folder moves inside
4. Drag onto same parent → nothing happens
5. Drag folder onto own child → nothing happens

**Step 5: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add components/doc-tree.tsx components/domain/doc-sidebar.tsx app/\(main\)/app-shell-wrapper.tsx
git commit -m "feat: implement internal drag-and-drop move for docs and folders"
```

---

### Task 9: Create FileDropZone for external .md file drops

Handle dragging `.md` files from the OS into the sidebar.

**Files:**
- Create: `components/file-drop-zone.tsx`
- Modify: `components/doc-tree.tsx` — integrate FileDropZone

**Step 1: Create the FileDropZone component**

Create `components/file-drop-zone.tsx`:

```tsx
"use client";

import { useCallback, useState, type DragEvent, type ReactNode } from "react";
import { filterMarkdownFiles, hasNameConflict, resolveDropPath } from "@/app/lib/knowhow/dnd-utils";
import { createDocument } from "@/app/lib/knowhow/mutations";
import { useToast } from "@/components/ui/toast-provider";
import type { DocumentSummary } from "@/app/lib/knowhow/types";

interface FileDropZoneProps {
  vaultId: string;
  targetFolderPath: string;
  documents: DocumentSummary[];
  onImportComplete: () => void;
  children: ReactNode;
}

export function FileDropZone({
  vaultId,
  targetFolderPath,
  documents,
  onImportComplete,
  children,
}: FileDropZoneProps) {
  const [isDragOver, setIsDragOver] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const { toast } = useToast();

  const handleDragEnter = useCallback((e: DragEvent) => {
    e.preventDefault();
    // Only activate for external files (not internal dnd-kit drags)
    if (e.dataTransfer.types.includes("Files")) {
      setIsDragOver(true);
    }
  }, []);

  const handleDragOver = useCallback((e: DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
  }, []);

  const handleDragLeave = useCallback((e: DragEvent) => {
    // Only deactivate when leaving the container (not entering a child)
    if (e.currentTarget === e.target) {
      setIsDragOver(false);
    }
  }, []);

  const handleDrop = useCallback(
    async (e: DragEvent) => {
      e.preventDefault();
      setIsDragOver(false);

      const files = Array.from(e.dataTransfer.files);
      const { valid, skipped } = filterMarkdownFiles(files);

      if (skipped > 0) {
        toast({
          variant: "info",
          title: `Skipped ${skipped} non-markdown file${skipped > 1 ? "s" : ""}`,
        });
      }

      if (valid.length === 0) return;

      setIsImporting(true);
      let imported = 0;
      let failed = 0;

      for (const file of valid) {
        const path = resolveDropPath(file.name, targetFolderPath);

        if (hasNameConflict(documents, path)) {
          toast({
            variant: "error",
            title: `"${file.name}" already exists in ${targetFolderPath || "root"}`,
          });
          failed++;
          continue;
        }

        const content = await file.text();
        const result = await createDocument(vaultId, path, content);

        if (result.success) {
          imported++;
        } else {
          toast({ variant: "error", title: `Failed to import ${file.name}` });
          failed++;
        }
      }

      setIsImporting(false);

      if (imported > 0) {
        toast({
          variant: "success",
          title: `Imported ${imported} file${imported > 1 ? "s" : ""}`,
        });
        onImportComplete();
      }
    },
    [vaultId, targetFolderPath, documents, toast, onImportComplete],
  );

  return (
    <div
      className="relative"
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {children}
      {isDragOver && (
        <div className="pointer-events-none absolute inset-0 z-40 flex items-center justify-center rounded-lg border-2 border-dashed border-blue-400 bg-blue-50/80 dark:border-blue-600 dark:bg-blue-950/80">
          <p className="text-sm font-medium text-blue-600 dark:text-blue-400">
            Drop .md files here
          </p>
        </div>
      )}
      {isImporting && (
        <div className="pointer-events-none absolute inset-0 z-40 flex items-center justify-center rounded-lg bg-white/60 dark:bg-zinc-900/60">
          <p className="text-sm text-zinc-500">Importing...</p>
        </div>
      )}
    </div>
  );
}
```

**Step 2: Integrate into DocTree**

In `doc-tree.tsx`, wrap the `DndContext` with `FileDropZone`:

```tsx
<FileDropZone
  vaultId={vaultId}
  targetFolderPath=""
  documents={documents}
  onImportComplete={() => router.refresh()}
>
  <DndContext ...>
    {/* existing tree */}
  </DndContext>
</FileDropZone>
```

Note: This provides a "root" drop target for external files over the entire sidebar. The per-folder targeting for external files would require more complex event coordination between native drag events and dnd-kit. Start with root-level external drops as the simpler version — users can drag internally to reorganize after import.

**Step 3: Verify in browser**

Test:
1. Drag a `.md` file from Finder onto sidebar → creates document
2. Drag a `.png` file → "Skipped 1 non-markdown file" toast
3. Drag multiple files → imports .md files, skips others
4. Drag file with duplicate name → error toast

**Step 4: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add components/file-drop-zone.tsx components/doc-tree.tsx
git commit -m "feat: add FileDropZone for importing .md files from OS"
```

---

### Task 10: Accessibility — dnd-kit announcements

Configure screen reader announcements for drag operations.

**Files:**
- Modify: `components/doc-tree.tsx`

**Step 1: Add announcements config**

```ts
import type { Announcements } from "@dnd-kit/core";

const announcements: Announcements = {
  onDragStart({ active }) {
    return `Picked up ${active.id}`;
  },
  onDragOver({ active, over }) {
    if (over) {
      const target = String(over.id).startsWith("drop:")
        ? String(over.id).slice(5) || "root"
        : over.id;
      return `${active.id} is over ${target}`;
    }
    return `${active.id} is no longer over a drop target`;
  },
  onDragEnd({ active, over }) {
    if (over) {
      const target = String(over.id).startsWith("drop:")
        ? String(over.id).slice(5) || "root"
        : over.id;
      return `${active.id} was dropped on ${target}`;
    }
    return `${active.id} was dropped`;
  },
  onDragCancel({ active }) {
    return `Dragging ${active.id} was cancelled`;
  },
};
```

**Step 2: Pass to DndContext**

```tsx
<DndContext
  sensors={sensors}
  accessibility={{ announcements }}
  onDragStart={handleDragStart}
  onDragOver={handleDragOver}
  onDragEnd={handleDragEnd}
>
```

**Step 3: Commit**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add components/doc-tree.tsx
git commit -m "feat: add screen reader announcements for drag operations"
```

---

### Task 11: Final integration test and cleanup

**Step 1: Run all tests**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run test:unit`
Expected: All tests pass

**Step 2: Run build**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run build`
Expected: Build succeeds with no type errors

**Step 3: Manual verification checklist**

Run: `cd /Users/raphaelgruber/Git/knowhow/main/web && bun run dev`

Test these scenarios:
- [ ] Drag document to different folder → moves
- [ ] Drag document to root zone → moves to root
- [ ] Drag folder into another folder → moves with children
- [ ] Drag folder into own child → rejected (no highlight)
- [ ] Drag to same parent → no-op
- [ ] Drag .md file from OS → creates document, success toast
- [ ] Drag .png from OS → "Skipped" toast
- [ ] Drag multiple files from OS → imports .md, skips others
- [ ] Name collision (internal) → error toast
- [ ] Name collision (external) → error toast
- [ ] Hover over collapsed folder 500ms → auto-expands
- [ ] Keyboard: Space to grab, arrows, Space to drop

**Step 4: Final commit if any fixes needed**

```bash
cd /Users/raphaelgruber/Git/knowhow/main/web
git add -A
git commit -m "fix: address integration test findings"
```
