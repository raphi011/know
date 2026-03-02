"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import {
  ChevronRightIcon,
  FolderIcon,
  DocumentTextIcon,
  DocumentPlusIcon,
  FolderPlusIcon,
  PencilIcon,
  TrashIcon,
} from "@heroicons/react/24/outline";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  ContextMenu,
  ContextMenuItem,
  ContextMenuSeparator,
} from "@/components/ui/context-menu";
import type { Position } from "@/components/ui/context-menu";
import { InlineTreeInput } from "@/components/inline-tree-input";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { cn } from "@/lib/utils";
import type { TreeNode } from "@/app/lib/knowhow/types";
import {
  createDocument,
  deleteDocument,
  moveDocument,
  deleteDocumentsByPrefix,
  moveDocumentsByPrefix,
} from "@/app/lib/knowhow/mutations";

type EditingState = {
  type: "new-doc" | "new-folder" | "rename";
  parentPath: string;
  currentName?: string;
  currentPath?: string;
};

type DocTreeProps = {
  tree: TreeNode[];
  activePath: string;
  vaultId: string;
};

function findNode(nodes: TreeNode[], path: string): TreeNode | undefined {
  for (const node of nodes) {
    if (node.path === path) return node;
    if (node.type === "folder") {
      const found = findNode(node.children, path);
      if (found) return found;
    }
  }
  return undefined;
}

function DocTree({ tree, activePath, vaultId }: DocTreeProps) {
  const router = useRouter();
  const t = useTranslations("tree");

  const [expanded, setExpanded] = useState<Set<string>>(() => {
    // Auto-expand folders that contain the active document
    const paths = new Set<string>();
    if (activePath) {
      const parts = activePath.split("/");
      for (let i = 1; i < parts.length; i++) {
        paths.add(parts.slice(0, i).join("/"));
      }
    }
    return paths;
  });

  // Context menu state
  const [contextMenu, setContextMenu] = useState<{
    position: Position;
    node: TreeNode | null; // null = right-click on empty area
  } | null>(null);

  // Inline editing state
  const [editing, setEditing] = useState<EditingState | null>(null);

  // Delete confirmation state
  const [deleteTarget, setDeleteTarget] = useState<TreeNode | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  function toggleFolder(path: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  }

  function handleContextMenu(e: React.MouseEvent, node: TreeNode | null) {
    e.preventDefault();
    e.stopPropagation();
    setContextMenu({ position: { x: e.clientX, y: e.clientY }, node });
  }

  function handleNewDocument(parentPath: string) {
    setContextMenu(null);
    setEditing({ type: "new-doc", parentPath });
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
    const currentName =
      node.type === "document"
        ? node.name.endsWith(".md")
          ? node.name
          : node.name + ".md"
        : node.name;
    setEditing({
      type: "rename",
      parentPath,
      currentName,
      currentPath: node.path,
    });
  }

  function handleDeleteRequest(node: TreeNode) {
    setContextMenu(null);
    setDeleteTarget(node);
    setDeleteError(null);
  }

  async function handleDeleteConfirm() {
    if (!deleteTarget) return;
    setDeleteLoading(true);
    setDeleteError(null);

    const result =
      deleteTarget.type === "folder"
        ? await deleteDocumentsByPrefix(vaultId, deleteTarget.path)
        : await deleteDocument(vaultId, deleteTarget.path);

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
      const path = editing.parentPath
        ? `${editing.parentPath}/${fullName}`
        : fullName;
      const result = await createDocument(vaultId, path, "");
      if (result.success) router.refresh();
    } else if (editing.type === "new-folder") {
      // Optimistic folder — just expand it, no server call
      const folderPath = editing.parentPath
        ? `${editing.parentPath}/${name}`
        : name;
      setExpanded((prev) => new Set([...prev, folderPath]));
    } else if (editing.type === "rename" && editing.currentPath) {
      const node = findNode(tree, editing.currentPath);
      const isFolder = node?.type === "folder";
      if (isFolder) {
        const newPath = editing.parentPath
          ? `${editing.parentPath}/${name}`
          : name;
        const result = await moveDocumentsByPrefix(
          vaultId,
          editing.currentPath,
          newPath,
        );
        if (result.success) router.refresh();
      } else {
        const newName = name.endsWith(".md") ? name : `${name}.md`;
        const newPath = editing.parentPath
          ? `${editing.parentPath}/${newName}`
          : newName;
        const result = await moveDocument(
          vaultId,
          editing.currentPath,
          newPath,
        );
        if (result.success) router.refresh();
      }
    }

    setEditing(null);
  }

  return (
    <>
      <ScrollArea className="h-full">
        <div
          className="min-h-full space-y-0.5 py-1"
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
          {editing && !editing.parentPath && editing.type !== "rename" && (
            <InlineTreeInput
              type={editing.type === "new-folder" ? "folder" : "document"}
              depth={0}
              siblingNames={tree.map((n) => n.name)}
              onConfirm={handleInlineConfirm}
              onCancel={() => setEditing(null)}
              placeholder={
                t(editing.type === "new-folder" ? "newFolder" : "newDocument")
              }
            />
          )}
        </div>
      </ScrollArea>

      {contextMenu && (
        <ContextMenu
          open
          position={contextMenu.position}
          onClose={() => setContextMenu(null)}
        >
          {contextMenu.node === null && (
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
          )}
          {contextMenu.node?.type === "folder" && (
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
                icon={<TrashIcon />}
                destructive
                onClick={() => handleDeleteRequest(contextMenu.node!)}
              >
                {t("delete")}
              </ContextMenuItem>
            </>
          )}
          {contextMenu.node?.type === "document" && (
            <>
              <ContextMenuItem
                icon={<PencilIcon />}
                onClick={() => handleRename(contextMenu.node!)}
              >
                {t("rename")}
              </ContextMenuItem>
              <ContextMenuItem
                icon={<TrashIcon />}
                destructive
                onClick={() => handleDeleteRequest(contextMenu.node!)}
              >
                {t("delete")}
              </ContextMenuItem>
            </>
          )}
        </ContextMenu>
      )}

      {deleteTarget && (
        <ConfirmDialog
          open
          onClose={() => setDeleteTarget(null)}
          onConfirm={handleDeleteConfirm}
          title={t("deleteConfirmTitle", { name: deleteTarget.name })}
          description={
            deleteTarget.type === "folder"
              ? t("deleteFolderConfirmDescription")
              : t("deleteConfirmDescription")
          }
          error={deleteError ?? undefined}
          loading={deleteLoading}
        />
      )}
    </>
  );
}

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
  editing: EditingState | null;
  onInlineConfirm: (name: string) => void;
  onInlineCancel: () => void;
}) {
  const isFolder = node.type === "folder";
  const isExpanded = expanded.has(node.path);
  const isActive = !isFolder && node.path === activePath;
  const isBeingRenamed =
    editing?.type === "rename" && editing.currentPath === node.path;

  if (isBeingRenamed) {
    return (
      <InlineTreeInput
        type={isFolder ? "folder" : "document"}
        depth={depth}
        defaultValue={editing!.currentName}
        siblingNames={[]}
        onConfirm={onInlineConfirm}
        onCancel={onInlineCancel}
      />
    );
  }

  const itemClasses = cn(
    "flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-sm",
    "transition-colors duration-100",
    isActive
      ? "bg-primary-50 text-primary-700 dark:bg-primary-950 dark:text-primary-400"
      : "text-slate-600 hover:bg-slate-50 dark:text-slate-400 dark:hover:bg-slate-800",
  );

  const itemContent = (
    <>
      {isFolder ? (
        <ChevronRightIcon
          className={cn(
            "size-3.5 shrink-0 transition-transform duration-150",
            isExpanded && "rotate-90",
          )}
        />
      ) : (
        <span className="size-3.5 shrink-0" />
      )}
      {isFolder ? (
        <FolderIcon className="size-4 shrink-0" />
      ) : (
        <DocumentTextIcon className="size-4 shrink-0" />
      )}
      <span className="truncate">{node.name}</span>
    </>
  );

  return (
    <>
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
          {editing &&
            editing.parentPath === node.path &&
            editing.type !== "rename" && (
              <InlineTreeInput
                type={editing.type === "new-folder" ? "folder" : "document"}
                depth={depth + 1}
                siblingNames={node.children.map((c) => c.name)}
                onConfirm={onInlineConfirm}
                onCancel={onInlineCancel}
              />
            )}
        </>
      )}
    </>
  );
}

export { DocTree };
export type { DocTreeProps };
