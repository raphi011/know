# Sidebar Context Menus — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add right-click context menus to the document tree sidebar with New Document, New Folder, Rename, and Delete actions.

**Architecture:** Backend gets two new batch mutations (deleteByPrefix, moveByPrefix) in the standard 3-layer pattern (GraphQL → Service → DB). Frontend adds a `ContextMenu` primitive (Headless UI), an `InlineTreeInput` composite, and wires them into `DocTree`. All name inputs are inline (VS Code style). Folders are virtual/optimistic.

**Tech Stack:** Go, SurrealDB, gqlgen, Next.js 16, Headless UI, Tailwind v4, next-intl

**Design doc:** `docs/plans/2026-03-02-sidebar-context-menus-design.md`

---

## Task 1: Backend — Batch delete by prefix (DB + Service)

**Files:**
- Modify: `internal/db/queries_document.go`
- Modify: `internal/document/service.go`
- Test: `internal/document/service_test.go`

### Step 1: Write failing test for `DeleteDocumentsByPrefix`

In `internal/document/service_test.go`, add a test. Since the DB layer needs SurrealDB (integration tests use testcontainers), first add the DB method and test it through the service in the integration tests. For unit-testable logic, test path normalization.

However, the primary test will be in `internal/integration/lifecycle_test.go` where SurrealDB is available. Add a subtest:

```go
t.Run("DeleteDocumentsByPrefix", func(t *testing.T) {
    // Create 3 docs: /guides/a.md, /guides/b.md, /other/c.md
    // Call DeleteDocumentsByPrefix(ctx, vaultID, "/guides")
    // Verify /guides/a.md and /guides/b.md are gone
    // Verify /other/c.md still exists
})
```

### Step 2: Add `DeleteDocumentsByPrefix` to DB client

In `internal/db/queries_document.go`:

```go
func (c *Client) DeleteDocumentsByPrefix(ctx context.Context, vaultID, pathPrefix string) (int, error) {
	sql := `
		LET $docs = SELECT id FROM document
			WHERE vault = type::record("vault", $vault_id)
			AND string::starts_with(path, $prefix);
		FOR $doc IN $docs {
			DELETE $doc.id;
		};
		RETURN array::len($docs);
	`
	results, err := surrealdb.Query[int](ctx, c.DB(), sql, map[string]any{
		"vault_id": bareID("vault", vaultID),
		"prefix":   pathPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("delete documents by prefix: %w", err)
	}
	if len(*results) == 0 || len((*results)[0].Result) == 0 {
		return 0, nil
	}
	return (*results)[0].Result[0], nil
}
```

Note: SurrealDB v3 syntax — verify `FOR $doc IN $docs` and `array::len` work. The `surrealdb` subagent should be consulted if these fail. Alternative: `DELETE document WHERE vault = ... AND string::starts_with(path, $prefix)` may work directly.

### Step 3: Add `DeleteByPrefix` to document service

In `internal/document/service.go`:

```go
func (s *Service) DeleteByPrefix(ctx context.Context, vaultID, pathPrefix string) (int, error) {
	prefix := models.NormalizePath(pathPrefix)
	// Ensure prefix ends with / to avoid matching /guides-extra when deleting /guides
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	count, err := s.db.DeleteDocumentsByPrefix(ctx, vaultID, prefix)
	if err != nil {
		return 0, fmt.Errorf("delete by prefix: %w", err)
	}
	return count, nil
}
```

### Step 4: Run tests

```bash
just test
```

### Step 5: Commit

```bash
git add internal/db/queries_document.go internal/document/service.go internal/integration/lifecycle_test.go
git commit -m "feat: add DeleteDocumentsByPrefix for batch folder deletion"
```

---

## Task 2: Backend — Batch move by prefix (DB + Service)

**Files:**
- Modify: `internal/db/queries_document.go`
- Modify: `internal/document/service.go`
- Test: `internal/integration/lifecycle_test.go`

### Step 1: Write failing test

In `internal/integration/lifecycle_test.go`:

```go
t.Run("MoveDocumentsByPrefix", func(t *testing.T) {
    // Create 2 docs: /old-folder/a.md, /old-folder/b.md
    // Call MoveByPrefix(ctx, vaultID, "/old-folder", "/new-folder")
    // Verify paths changed to /new-folder/a.md, /new-folder/b.md
})
```

### Step 2: Add `MoveDocumentsByPrefix` to DB client

In `internal/db/queries_document.go`:

```go
func (c *Client) MoveDocumentsByPrefix(ctx context.Context, vaultID, oldPrefix, newPrefix string) (int, error) {
	sql := `
		LET $docs = SELECT id, path FROM document
			WHERE vault = type::record("vault", $vault_id)
			AND string::starts_with(path, $old_prefix);
		FOR $doc IN $docs {
			UPDATE $doc.id SET path = string::concat($new_prefix, string::slice($doc.path, string::len($old_prefix)));
		};
		RETURN array::len($docs);
	`
	results, err := surrealdb.Query[int](ctx, c.DB(), sql, map[string]any{
		"vault_id":   bareID("vault", vaultID),
		"old_prefix": oldPrefix,
		"new_prefix": newPrefix,
	})
	if err != nil {
		return 0, fmt.Errorf("move documents by prefix: %w", err)
	}
	if len(*results) == 0 || len((*results)[0].Result) == 0 {
		return 0, nil
	}
	return (*results)[0].Result[0], nil
}
```

### Step 3: Add `MoveByPrefix` to document service

In `internal/document/service.go`:

```go
func (s *Service) MoveByPrefix(ctx context.Context, vaultID, oldPrefix, newPrefix string) (int, error) {
	old := models.NormalizePath(oldPrefix)
	if !strings.HasSuffix(old, "/") {
		old += "/"
	}
	new := models.NormalizePath(newPrefix)
	if !strings.HasSuffix(new, "/") {
		new += "/"
	}
	count, err := s.db.MoveDocumentsByPrefix(ctx, vaultID, old, new)
	if err != nil {
		return 0, fmt.Errorf("move by prefix: %w", err)
	}
	return count, nil
}
```

### Step 4: Run tests

```bash
just test
```

### Step 5: Commit

```bash
git add internal/db/queries_document.go internal/document/service.go internal/integration/lifecycle_test.go
git commit -m "feat: add MoveDocumentsByPrefix for batch folder rename"
```

---

## Task 3: Backend — GraphQL mutations for batch operations

**Files:**
- Modify: `internal/graph/schema.graphqls`
- Modify: `internal/graph/schema.resolvers.go` (auto-generated stubs, then implement)

### Step 1: Add mutations to schema

In `internal/graph/schema.graphqls`, add to the `Mutation` type (after existing document mutations):

```graphql
  deleteDocumentsByPrefix(vaultId: ID!, pathPrefix: String!): Int!
  moveDocumentsByPrefix(vaultId: ID!, oldPrefix: String!, newPrefix: String!): Int!
```

### Step 2: Regenerate gqlgen code

```bash
just generate
```

This creates resolver stubs in `schema.resolvers.go`.

### Step 3: Implement resolvers

In `internal/graph/schema.resolvers.go`, fill in the generated stubs:

```go
func (r *mutationResolver) DeleteDocumentsByPrefix(ctx context.Context, vaultID string, pathPrefix string) (int, error) {
	if err := auth.RequireVaultAccess(ctx, vaultID); err != nil {
		return 0, err
	}
	return r.documentService.DeleteByPrefix(ctx, vaultID, pathPrefix)
}

func (r *mutationResolver) MoveDocumentsByPrefix(ctx context.Context, vaultID string, oldPrefix string, newPrefix string) (int, error) {
	if err := auth.RequireVaultAccess(ctx, vaultID); err != nil {
		return 0, err
	}
	return r.documentService.MoveByPrefix(ctx, vaultID, oldPrefix, newPrefix)
}
```

### Step 4: Build and test

```bash
just build-all && just test
```

### Step 5: Commit

```bash
git add internal/graph/schema.graphqls internal/graph/schema.resolvers.go internal/graph/generated.go
git commit -m "feat: add GraphQL mutations for batch folder delete/move"
```

---

## Task 4: Frontend — i18n strings

**Files:**
- Modify: `web/messages/en.json`
- Modify: `web/messages/de.json`

### Step 1: Add tree namespace to English locale

In `web/messages/en.json`, add after the `"docs"` section:

```json
"tree": {
  "newDocument": "New document",
  "newFolder": "New folder",
  "rename": "Rename",
  "delete": "Delete",
  "deleteConfirmTitle": "Delete \"{name}\"?",
  "deleteConfirmDescription": "This action cannot be undone.",
  "deleteFolderConfirmDescription": "This will permanently delete all documents in this folder.",
  "nameRequired": "Name is required",
  "nameInvalid": "Name cannot contain /",
  "nameDuplicate": "An item with this name already exists",
  "nameInputLabel": "Item name"
}
```

### Step 2: Add tree namespace to German locale

In `web/messages/de.json`, add the same keys with German translations:

```json
"tree": {
  "newDocument": "Neues Dokument",
  "newFolder": "Neuer Ordner",
  "rename": "Umbenennen",
  "delete": "Löschen",
  "deleteConfirmTitle": "\"{name}\" löschen?",
  "deleteConfirmDescription": "Diese Aktion kann nicht rückgängig gemacht werden.",
  "deleteFolderConfirmDescription": "Alle Dokumente in diesem Ordner werden unwiderruflich gelöscht.",
  "nameRequired": "Name ist erforderlich",
  "nameInvalid": "Name darf kein / enthalten",
  "nameDuplicate": "Ein Element mit diesem Namen existiert bereits",
  "nameInputLabel": "Name des Elements"
}
```

### Step 3: Verify web builds

```bash
just web-build
```

### Step 4: Commit

```bash
git add web/messages/en.json web/messages/de.json
git commit -m "feat: add i18n strings for tree context menu actions"
```

---

## Task 5: Frontend — ContextMenu primitive

**Files:**
- Create: `web/components/ui/context-menu.tsx`
- Create: `web/stories/ui/ContextMenu.stories.tsx`

### Step 1: Create the ContextMenu component

Create `web/components/ui/context-menu.tsx`. This wraps Headless UI's `Menu` but positions at cursor coordinates using an invisible anchor:

```tsx
"use client";

import { Fragment, useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import {
  Menu,
  MenuButton,
  MenuItem,
  MenuItems,
  Transition,
} from "@headlessui/react";
import { cn } from "@/lib/utils";

type Position = { x: number; y: number };

type ContextMenuProps = {
  open: boolean;
  position: Position;
  onClose: () => void;
  children: React.ReactNode;
};

function ContextMenu({ open, position, onClose, children }: ContextMenuProps) {
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handleClickOutside(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    }
    function handleScroll() {
      onClose();
    }
    // Delay to avoid closing immediately from the right-click event
    const id = requestAnimationFrame(() => {
      document.addEventListener("mousedown", handleClickOutside);
      document.addEventListener("scroll", handleScroll, true);
    });
    return () => {
      cancelAnimationFrame(id);
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("scroll", handleScroll, true);
    };
  }, [open, onClose]);

  useEffect(() => {
    if (!open) return;
    function handleEscape(e: KeyboardEvent) {
      if (e.key === "Escape") {
        onClose();
      }
    }
    document.addEventListener("keydown", handleEscape);
    return () => document.removeEventListener("keydown", handleEscape);
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <div
      ref={menuRef}
      className={cn(
        "fixed z-50 min-w-[180px] rounded-xl bg-white p-1 shadow-md",
        "ring-1 ring-slate-200",
        "dark:bg-slate-900 dark:ring-slate-800",
        "animate-in fade-in zoom-in-95 duration-150",
      )}
      style={{ top: position.y, left: position.x }}
      role="menu"
      tabIndex={-1}
    >
      {children}
    </div>,
    document.body,
  );
}

type ContextMenuItemProps = {
  children: React.ReactNode;
  onClick?: () => void;
  destructive?: boolean;
  icon?: React.ReactNode;
  disabled?: boolean;
};

function ContextMenuItem({
  children,
  onClick,
  destructive,
  icon,
  disabled,
}: ContextMenuItemProps) {
  return (
    <button
      role="menuitem"
      onClick={(e) => {
        e.stopPropagation();
        onClick?.();
      }}
      disabled={disabled}
      className={cn(
        "flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm",
        "transition-colors duration-100",
        destructive
          ? "text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950"
          : "text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800",
        disabled && "cursor-not-allowed opacity-50",
      )}
    >
      {icon && <span className="shrink-0 [&_svg]:size-4">{icon}</span>}
      {children}
    </button>
  );
}

function ContextMenuSeparator() {
  return <div className="my-1 h-px bg-slate-200 dark:bg-slate-800" />;
}

export { ContextMenu, ContextMenuItem, ContextMenuSeparator };
export type { ContextMenuProps, ContextMenuItemProps, Position };
```

Note: This uses a simpler approach than wrapping Headless UI `Menu` — a portal-rendered div with manual close handlers. This avoids the adapter complexity of making `Menu` work with arbitrary coordinates while still matching the exact same visual styles as `DropdownMenuItem`. Arrow key navigation can be added later if needed; Escape and click-outside are handled.

### Step 2: Write Storybook story

Create `web/stories/ui/ContextMenu.stories.tsx`:

```tsx
import { useState } from "react";
import preview from "#.storybook/preview";
import {
  ContextMenu,
  ContextMenuItem,
  ContextMenuSeparator,
} from "@/components/ui/context-menu";
import {
  DocumentPlusIcon,
  FolderPlusIcon,
  PencilIcon,
  TrashIcon,
} from "@heroicons/react/24/outline";

const meta = preview.meta({
  title: "UI/ContextMenu",
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
});

export default meta;

export const Default = meta.story({
  render: () => {
    const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);

    return (
      <div
        className="flex h-96 items-center justify-center bg-slate-50 dark:bg-slate-950"
        onContextMenu={(e) => {
          e.preventDefault();
          setMenu({ x: e.clientX, y: e.clientY });
        }}
      >
        <p className="text-sm text-slate-500">Right-click anywhere</p>
        {menu && (
          <ContextMenu open position={menu} onClose={() => setMenu(null)}>
            <ContextMenuItem
              icon={<DocumentPlusIcon />}
              onClick={() => setMenu(null)}
            >
              New document
            </ContextMenuItem>
            <ContextMenuItem
              icon={<FolderPlusIcon />}
              onClick={() => setMenu(null)}
            >
              New folder
            </ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem
              icon={<PencilIcon />}
              onClick={() => setMenu(null)}
            >
              Rename
            </ContextMenuItem>
            <ContextMenuItem
              destructive
              icon={<TrashIcon />}
              onClick={() => setMenu(null)}
            >
              Delete
            </ContextMenuItem>
          </ContextMenu>
        )}
      </div>
    );
  },
});
```

### Step 3: Verify in Storybook

```bash
cd web && bun storybook
```

Open `http://localhost:6006` → UI → ContextMenu → right-click in the story.

### Step 4: Commit

```bash
git add web/components/ui/context-menu.tsx web/stories/ui/ContextMenu.stories.tsx
git commit -m "feat: add ContextMenu UI primitive"
```

---

## Task 6: Frontend — InlineTreeInput component

**Files:**
- Create: `web/components/inline-tree-input.tsx`

### Step 1: Create the InlineTreeInput component

This is a small text input that appears inline in the tree at the correct indentation level. It handles Enter/Escape, validation, and auto-focus.

Create `web/components/inline-tree-input.tsx`:

```tsx
"use client";

import { useEffect, useRef, useState } from "react";
import {
  DocumentTextIcon,
  FolderIcon,
} from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";

type InlineTreeInputProps = {
  type: "document" | "folder";
  depth: number;
  defaultValue?: string;
  siblingNames: string[];
  onConfirm: (name: string) => void;
  onCancel: () => void;
  placeholder?: string;
};

function InlineTreeInput({
  type,
  depth,
  defaultValue = "",
  siblingNames,
  onConfirm,
  onCancel,
  placeholder,
}: InlineTreeInputProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState(defaultValue);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    input.focus();
    if (defaultValue) {
      // Select the name part before the extension
      const dotIndex = defaultValue.lastIndexOf(".");
      input.setSelectionRange(0, dotIndex > 0 ? dotIndex : defaultValue.length);
    }
  }, [defaultValue]);

  function validate(name: string): string | null {
    const trimmed = name.trim();
    if (!trimmed) return "nameRequired";
    if (trimmed.includes("/")) return "nameInvalid";
    const compareName = type === "document" && !trimmed.endsWith(".md")
      ? `${trimmed}.md`
      : trimmed;
    if (siblingNames.some((s) => s.toLowerCase() === compareName.toLowerCase())) {
      return "nameDuplicate";
    }
    return null;
  }

  function handleSubmit() {
    const trimmed = value.trim();
    const validationError = validate(trimmed);
    if (validationError) {
      setError(validationError);
      return;
    }
    onConfirm(trimmed);
  }

  return (
    <div
      className="flex w-full items-center gap-2 px-2 py-0.5"
      style={{ paddingLeft: `${depth * 16 + 8}px` }}
    >
      <span className="size-3.5 shrink-0" />
      {type === "folder" ? (
        <FolderIcon className="size-4 shrink-0 text-slate-400" />
      ) : (
        <DocumentTextIcon className="size-4 shrink-0 text-slate-400" />
      )}
      <input
        ref={inputRef}
        type="text"
        value={value}
        onChange={(e) => {
          setValue(e.target.value);
          setError(null);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            handleSubmit();
          } else if (e.key === "Escape") {
            e.preventDefault();
            onCancel();
          }
        }}
        onBlur={onCancel}
        placeholder={placeholder}
        aria-label={placeholder ?? "Item name"}
        className={cn(
          "min-w-0 flex-1 rounded border bg-white px-1.5 py-0.5 text-sm outline-none",
          "dark:bg-slate-900",
          error
            ? "border-red-400 focus:ring-1 focus:ring-red-400"
            : "border-slate-300 focus:border-primary-400 focus:ring-1 focus:ring-primary-400 dark:border-slate-700",
        )}
      />
    </div>
  );
}

export { InlineTreeInput };
export type { InlineTreeInputProps };
```

### Step 2: Verify it builds

```bash
just web-build
```

### Step 3: Commit

```bash
git add web/components/inline-tree-input.tsx
git commit -m "feat: add InlineTreeInput component for tree name editing"
```

---

## Task 7: Frontend — Server actions for document CRUD

**Files:**
- Modify: `web/app/lib/knowhow/mutations.ts`

### Step 1: Add new mutation functions

Add to `web/app/lib/knowhow/mutations.ts` alongside the existing `saveDocument`:

```ts
export async function createDocument(
  vaultId: string,
  path: string,
  content: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $file: FileInput!) {
            createDocument(vaultId: $vaultId, file: $file) { id }
          }
        `,
        variables: { vaultId, file: { path, content } },
      }),
    });

    if (!response.ok) {
      return { success: false, error: `HTTP ${response.status}` };
    }

    const json = await response.json();
    if (json.errors?.length) {
      return { success: false, error: json.errors[0].message };
    }
    return { success: true };
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return { success: false, error: message };
  }
}

export async function deleteDocument(
  vaultId: string,
  path: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $path: String!) {
            deleteDocument(vaultId: $vaultId, path: $path)
          }
        `,
        variables: { vaultId, path },
      }),
    });

    if (!response.ok) {
      return { success: false, error: `HTTP ${response.status}` };
    }

    const json = await response.json();
    if (json.errors?.length) {
      return { success: false, error: json.errors[0].message };
    }
    return { success: true };
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return { success: false, error: message };
  }
}

export async function moveDocument(
  vaultId: string,
  oldPath: string,
  newPath: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $oldPath: String!, $newPath: String!) {
            moveDocument(vaultId: $vaultId, oldPath: $oldPath, newPath: $newPath) { id }
          }
        `,
        variables: { vaultId, oldPath, newPath },
      }),
    });

    if (!response.ok) {
      return { success: false, error: `HTTP ${response.status}` };
    }

    const json = await response.json();
    if (json.errors?.length) {
      return { success: false, error: json.errors[0].message };
    }
    return { success: true };
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return { success: false, error: message };
  }
}

export async function deleteDocumentsByPrefix(
  vaultId: string,
  pathPrefix: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $pathPrefix: String!) {
            deleteDocumentsByPrefix(vaultId: $vaultId, pathPrefix: $pathPrefix)
          }
        `,
        variables: { vaultId, pathPrefix },
      }),
    });

    if (!response.ok) {
      return { success: false, error: `HTTP ${response.status}` };
    }

    const json = await response.json();
    if (json.errors?.length) {
      return { success: false, error: json.errors[0].message };
    }
    return { success: true };
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return { success: false, error: message };
  }
}

export async function moveDocumentsByPrefix(
  vaultId: string,
  oldPrefix: string,
  newPrefix: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $oldPrefix: String!, $newPrefix: String!) {
            moveDocumentsByPrefix(vaultId: $vaultId, oldPrefix: $oldPrefix, newPrefix: $newPrefix)
          }
        `,
        variables: { vaultId, oldPrefix, newPrefix },
      }),
    });

    if (!response.ok) {
      return { success: false, error: `HTTP ${response.status}` };
    }

    const json = await response.json();
    if (json.errors?.length) {
      return { success: false, error: json.errors[0].message };
    }
    return { success: true };
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return { success: false, error: message };
  }
}
```

Note: There's clear duplication in these functions. Consider extracting a shared `graphqlMutation` helper after wiring everything up (Task 10). For now, keep them explicit so each can be tested and debugged independently.

### Step 2: Verify build

```bash
just web-build
```

### Step 3: Commit

```bash
git add web/app/lib/knowhow/mutations.ts
git commit -m "feat: add client-side mutation functions for document CRUD"
```

---

## Task 8: Frontend — Thread vaultId to DocTree

**Files:**
- Modify: `web/app/(main)/app-shell-wrapper.tsx`
- Modify: `web/components/domain/doc-sidebar.tsx`
- Modify: `web/components/doc-tree.tsx`

### Step 1: Pass vaultId from AppShellWrapper → DocSidebar

In `web/app/(main)/app-shell-wrapper.tsx`, change:

```tsx
{vault && <DocSidebar tree={tree} />}
```

To:

```tsx
{vault && <DocSidebar tree={tree} vaultId={vault.id} />}
```

### Step 2: Update DocSidebar to accept and pass vaultId

In `web/components/domain/doc-sidebar.tsx`:

```tsx
type DocSidebarProps = {
  tree: TreeNode[];
  vaultId: string;
};

function DocSidebar({ tree, vaultId }: DocSidebarProps) {
  const pathname = usePathname();
  const activePath = pathname.startsWith("/docs/")
    ? pathname.slice("/docs/".length)
    : "";

  return <DocTree tree={tree} activePath={activePath} vaultId={vaultId} />;
}
```

### Step 3: Update DocTree props to accept vaultId

In `web/components/doc-tree.tsx`, update `DocTreeProps`:

```tsx
type DocTreeProps = {
  tree: TreeNode[];
  activePath: string;
  vaultId: string;
};
```

And the function signature:

```tsx
function DocTree({ tree, activePath, vaultId }: DocTreeProps) {
```

(The vaultId will be used in the next task when wiring up context menus.)

### Step 4: Verify build

```bash
just web-build
```

### Step 5: Commit

```bash
git add web/app/(main)/app-shell-wrapper.tsx web/components/domain/doc-sidebar.tsx web/components/doc-tree.tsx
git commit -m "feat: thread vaultId from AppShellWrapper through to DocTree"
```

---

## Task 9: Frontend — Wire context menus into DocTree

This is the largest task. It connects the ContextMenu, InlineTreeInput, and mutations into the existing DocTree.

**Files:**
- Modify: `web/components/doc-tree.tsx`

### Step 1: Add state and imports to DocTree

Add imports at the top of `doc-tree.tsx`:

```tsx
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import {
  DocumentPlusIcon,
  FolderPlusIcon,
  PencilIcon,
  TrashIcon,
} from "@heroicons/react/24/outline";
import { ContextMenu, ContextMenuItem, ContextMenuSeparator } from "@/components/ui/context-menu";
import type { Position } from "@/components/ui/context-menu";
import { InlineTreeInput } from "@/components/inline-tree-input";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  createDocument,
  deleteDocument,
  moveDocument,
  deleteDocumentsByPrefix,
  moveDocumentsByPrefix,
} from "@/app/lib/knowhow/mutations";
```

### Step 2: Add context menu state to DocTree function

Inside `DocTree`, add state after the existing `expanded` state:

```tsx
const router = useRouter();
const t = useTranslations("tree");

// Context menu state
const [contextMenu, setContextMenu] = useState<{
  position: Position;
  node: TreeNode | null; // null = empty area
} | null>(null);

// Inline editing state
const [editing, setEditing] = useState<{
  type: "new-doc" | "new-folder" | "rename";
  parentPath: string;
  currentName?: string;
  currentPath?: string;
} | null>(null);

// Delete confirmation state
const [deleteTarget, setDeleteTarget] = useState<{
  node: TreeNode;
} | null>(null);
const [deleteLoading, setDeleteLoading] = useState(false);
const [deleteError, setDeleteError] = useState<string | null>(null);
```

### Step 3: Add context menu handler

Add a `handleContextMenu` function in DocTree:

```tsx
function handleContextMenu(e: React.MouseEvent, node: TreeNode | null) {
  e.preventDefault();
  e.stopPropagation();
  setContextMenu({ position: { x: e.clientX, y: e.clientY }, node });
}
```

### Step 4: Add action handlers

```tsx
function handleNewDocument(parentPath: string) {
  setContextMenu(null);
  setEditing({ type: "new-doc", parentPath });
  // Auto-expand the parent folder
  if (parentPath) {
    setExpanded((prev) => new Set([...prev, parentPath]));
  }
}

function handleNewFolder(parentPath: string) {
  setContextMenu(null);
  setEditing({ type: "new-folder", parentPath });
  if (parentPath) {
    setExpanded((prev) => new Set([...prev, parentPath]));
  }
}

function handleRename(node: TreeNode) {
  setContextMenu(null);
  const parentPath = node.path.includes("/")
    ? node.path.substring(0, node.path.lastIndexOf("/"))
    : "";
  setEditing({
    type: "rename",
    parentPath,
    currentName: node.name + (node.type === "document" && !node.name.endsWith(".md") ? ".md" : ""),
    currentPath: node.path,
  });
}

function handleDeleteRequest(node: TreeNode) {
  setContextMenu(null);
  setDeleteTarget({ node });
  setDeleteError(null);
}

async function handleDeleteConfirm() {
  if (!deleteTarget) return;
  setDeleteLoading(true);
  setDeleteError(null);

  const { node } = deleteTarget;
  const result = node.type === "folder"
    ? await deleteDocumentsByPrefix(vaultId, node.path)
    : await deleteDocument(vaultId, node.path);

  setDeleteLoading(false);
  if (result.success) {
    setDeleteTarget(null);
    router.refresh();
  } else {
    setDeleteError(result.error);
  }
}

async function handleInlineConfirm(name: string) {
  if (!editing) return;

  if (editing.type === "new-doc") {
    const fullName = name.endsWith(".md") ? name : `${name}.md`;
    const path = editing.parentPath ? `${editing.parentPath}/${fullName}` : fullName;
    const result = await createDocument(vaultId, path, "");
    if (result.success) {
      router.refresh();
    }
  } else if (editing.type === "new-folder") {
    // Optimistic folder — just expand it, no server call
    // The folder will appear when a doc is created inside
    // For now, we store it as an optimistic expanded folder
    const folderPath = editing.parentPath ? `${editing.parentPath}/${name}` : name;
    setExpanded((prev) => new Set([...prev, folderPath]));
  } else if (editing.type === "rename" && editing.currentPath) {
    const isFolder = !editing.currentPath.includes(".");
    if (isFolder) {
      const parentPath = editing.currentPath.includes("/")
        ? editing.currentPath.substring(0, editing.currentPath.lastIndexOf("/"))
        : "";
      const newPath = parentPath ? `${parentPath}/${name}` : name;
      const result = await moveDocumentsByPrefix(vaultId, editing.currentPath, newPath);
      if (result.success) router.refresh();
    } else {
      const parentPath = editing.parentPath;
      const newName = name.endsWith(".md") ? name : `${name}.md`;
      const newPath = parentPath ? `${parentPath}/${newName}` : newName;
      const result = await moveDocument(vaultId, editing.currentPath, newPath);
      if (result.success) router.refresh();
    }
  }

  setEditing(null);
}
```

### Step 5: Add context menu rendering and empty-area handler to JSX

Wrap the existing `ScrollArea` return with context menu handling. The outer div handles right-click on empty area:

```tsx
return (
  <>
    <ScrollArea className="h-full">
      <div
        className="space-y-0.5 py-1"
        onContextMenu={(e) => handleContextMenu(e, null)}
      >
        {tree.map((node) => (
          <TreeNodeItem
            key={node.path}
            node={node}
            depth={0}
            activePath={activePath}
            expanded={expanded}
            onToggle={toggleFolder}
            onContextMenu={handleContextMenu}
            editing={editing}
            onInlineConfirm={handleInlineConfirm}
            onInlineCancel={() => setEditing(null)}
          />
        ))}
        {/* Inline input at root level for new items */}
        {editing && !editing.parentPath && (
          <InlineTreeInput
            type={editing.type === "new-folder" ? "folder" : "document"}
            depth={0}
            defaultValue={editing.currentName}
            siblingNames={tree.map((n) => n.name)}
            onConfirm={handleInlineConfirm}
            onCancel={() => setEditing(null)}
            placeholder={t(editing.type === "new-folder" ? "newFolder" : "newDocument")}
          />
        )}
      </div>
    </ScrollArea>

    {/* Context menu */}
    {contextMenu && (
      <ContextMenu
        open
        position={contextMenu.position}
        onClose={() => setContextMenu(null)}
      >
        {contextMenu.node === null ? (
          <>
            <ContextMenuItem
              icon={<DocumentPlusIcon />}
              onClick={() => handleNewDocument("")}
            >
              {t("newDocument")}
            </ContextMenuItem>
            <ContextMenuItem
              icon={<FolderPlusIcon />}
              onClick={() => handleNewFolder("")}
            >
              {t("newFolder")}
            </ContextMenuItem>
          </>
        ) : contextMenu.node.type === "folder" ? (
          <>
            <ContextMenuItem
              icon={<DocumentPlusIcon />}
              onClick={() => handleNewDocument(contextMenu.node!.path)}
            >
              {t("newDocument")}
            </ContextMenuItem>
            <ContextMenuItem
              icon={<FolderPlusIcon />}
              onClick={() => handleNewFolder(contextMenu.node!.path)}
            >
              {t("newFolder")}
            </ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem
              icon={<PencilIcon />}
              onClick={() => handleRename(contextMenu.node!)}
            >
              {t("rename")}
            </ContextMenuItem>
            <ContextMenuItem
              destructive
              icon={<TrashIcon />}
              onClick={() => handleDeleteRequest(contextMenu.node!)}
            >
              {t("delete")}
            </ContextMenuItem>
          </>
        ) : (
          <>
            <ContextMenuItem
              icon={<PencilIcon />}
              onClick={() => handleRename(contextMenu.node!)}
            >
              {t("rename")}
            </ContextMenuItem>
            <ContextMenuItem
              destructive
              icon={<TrashIcon />}
              onClick={() => handleDeleteRequest(contextMenu.node!)}
            >
              {t("delete")}
            </ContextMenuItem>
          </>
        )}
      </ContextMenu>
    )}

    {/* Delete confirmation dialog */}
    {deleteTarget && (
      <ConfirmDialog
        open
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDeleteConfirm}
        title={t("deleteConfirmTitle", { name: deleteTarget.node.name })}
        description={
          deleteTarget.node.type === "folder"
            ? t("deleteFolderConfirmDescription")
            : t("deleteConfirmDescription")
        }
        error={deleteError ?? undefined}
        loading={deleteLoading}
      />
    )}
  </>
);
```

### Step 6: Update TreeNodeItem to support context menu and inline editing

Update the `TreeNodeItem` props to accept the new handlers:

```tsx
function TreeNodeItem({
  node,
  depth,
  activePath,
  expanded,
  onToggle,
  onContextMenu,
  editing,
  onInlineConfirm,
  onInlineCancel,
}: {
  node: TreeNode;
  depth: number;
  activePath: string;
  expanded: Set<string>;
  onToggle: (path: string) => void;
  onContextMenu: (e: React.MouseEvent, node: TreeNode) => void;
  editing: { type: "new-doc" | "new-folder" | "rename"; parentPath: string; currentName?: string; currentPath?: string } | null;
  onInlineConfirm: (name: string) => void;
  onInlineCancel: () => void;
}) {
```

Add `onContextMenu` to the button/link elements:

```tsx
{isFolder ? (
  <button
    onClick={() => onToggle(node.path)}
    onContextMenu={(e) => onContextMenu(e, node)}
    className={itemClasses}
    style={{ paddingLeft: `${depth * 16 + 8}px` }}
  >
    {itemContent}
  </button>
) : (
  <Link
    href={`/docs/${node.path}`}
    onContextMenu={(e) => onContextMenu(e, node)}
    className={itemClasses}
    style={{ paddingLeft: `${depth * 16 + 8}px` }}
    aria-current={isActive ? "page" : undefined}
  >
    {itemContent}
  </Link>
)}
```

If `editing.type === "rename"` and `editing.currentPath === node.path`, render InlineTreeInput instead of the normal node. Add this check before the normal rendering:

```tsx
const isRenaming = editing?.type === "rename" && editing.currentPath === node.path;

if (isRenaming) {
  return (
    <InlineTreeInput
      type={isFolder ? "folder" : "document"}
      depth={depth}
      defaultValue={editing!.currentName}
      siblingNames={[]} // Siblings determined at parent level
      onConfirm={onInlineConfirm}
      onCancel={onInlineCancel}
      placeholder={node.name}
    />
  );
}
```

After rendering children of an expanded folder, add the inline input for new items inside this folder:

```tsx
{node.type === "folder" && isExpanded && (
  <>
    {node.children.map((child) => (
      <TreeNodeItem
        key={child.path}
        node={child}
        depth={depth + 1}
        activePath={activePath}
        expanded={expanded}
        onToggle={onToggle}
        onContextMenu={onContextMenu}
        editing={editing}
        onInlineConfirm={onInlineConfirm}
        onInlineCancel={onInlineCancel}
      />
    ))}
    {editing && editing.parentPath === node.path && editing.type !== "rename" && (
      <InlineTreeInput
        type={editing.type === "new-folder" ? "folder" : "document"}
        depth={depth + 1}
        siblingNames={node.children.map((c) => c.name)}
        onConfirm={onInlineConfirm}
        onCancel={onInlineCancel}
        placeholder={editing.type === "new-folder" ? "New folder" : "New document"}
      />
    )}
  </>
)}
```

### Step 7: Verify build

```bash
just web-build
```

### Step 8: Commit

```bash
git add web/components/doc-tree.tsx
git commit -m "feat: wire context menus, inline editing, and mutations into DocTree"
```

---

## Task 10: Refactor — Extract shared GraphQL mutation helper

**Files:**
- Modify: `web/app/lib/knowhow/mutations.ts`

### Step 1: Extract helper

The 6 mutation functions in `mutations.ts` share identical error-handling boilerplate. Extract:

```ts
async function graphqlMutation(
  query: string,
  variables: Record<string, unknown>,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query, variables }),
    });

    if (!response.ok) {
      return { success: false, error: `HTTP ${response.status}` };
    }

    let json: { errors?: { message: string }[] };
    try {
      json = await response.json();
    } catch {
      return { success: false, error: "Server returned an invalid response" };
    }

    if (json.errors?.length) {
      return { success: false, error: json.errors[0]!.message };
    }

    return { success: true };
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return { success: false, error: message };
  }
}
```

Then simplify each function to use it:

```ts
export function createDocument(vaultId: string, path: string, content: string) {
  return graphqlMutation(
    `mutation ($vaultId: ID!, $file: FileInput!) {
      createDocument(vaultId: $vaultId, file: $file) { id }
    }`,
    { vaultId, file: { path, content } },
  );
}
```

### Step 2: Verify build and lint

```bash
just web-build && cd web && bun run lint
```

### Step 3: Commit

```bash
git add web/app/lib/knowhow/mutations.ts
git commit -m "refactor: extract shared graphqlMutation helper in mutations.ts"
```

---

## Task 11: Testing — Storybook story for DocTree with context menu

**Files:**
- Create or modify: `web/stories/composites/DocTree.stories.tsx`

### Step 1: Write DocTree story with context menu

Create a story that shows the tree with context menu interaction:

```tsx
import { useState } from "react";
import preview from "#.storybook/preview";
import { DocTree } from "@/components/doc-tree";
import type { TreeNode } from "@/app/lib/knowhow/types";

const sampleTree: TreeNode[] = [
  {
    type: "folder",
    name: "guides",
    path: "guides",
    children: [
      { type: "document", name: "getting-started", path: "guides/getting-started.md" },
      { type: "document", name: "advanced", path: "guides/advanced.md" },
    ],
  },
  {
    type: "folder",
    name: "api",
    path: "api",
    children: [
      { type: "document", name: "endpoints", path: "api/endpoints.md" },
    ],
  },
  { type: "document", name: "README", path: "README.md" },
];

const meta = preview.meta({
  title: "Domain/DocTree",
  component: DocTree,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
});

export default meta;

export const WithContextMenu = meta.story({
  args: {
    tree: sampleTree,
    activePath: "guides/getting-started.md",
    vaultId: "vault:test",
  },
  render: (args) => (
    <div className="h-96 w-64 border border-slate-200 dark:border-slate-800">
      <DocTree {...args} />
    </div>
  ),
});
```

### Step 2: Verify in Storybook

```bash
cd web && bun storybook
```

### Step 3: Commit

```bash
git add web/stories/composites/DocTree.stories.tsx
git commit -m "feat: add DocTree Storybook story with context menu demo"
```

---

## Task 12: End-to-end verification

### Step 1: Run full backend tests

```bash
just test
```

### Step 2: Run full frontend build + lint + tests

```bash
just web-build && cd web && bun run lint && bun run test:unit
```

### Step 3: Manual smoke test

```bash
just dev-all
```

Open `http://localhost:3000`, navigate to docs:
- Right-click a folder → verify context menu shows New Document, New Folder, Rename, Delete
- Right-click a document → verify context menu shows Rename, Delete
- Right-click empty area → verify New Document, New Folder
- Test "New Document" → inline input → Enter → document created
- Test "Rename" → inline input pre-filled → Enter → document renamed
- Test "Delete" → confirmation dialog → confirm → document deleted

### Step 4: Final commit if any fixes needed

```bash
git add -A && git commit -m "fix: address smoke test findings"
```
