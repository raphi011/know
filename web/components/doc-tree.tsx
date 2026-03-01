"use client";

import { useState } from "react";
import Link from "next/link";
import {
  ChevronRightIcon,
  FolderIcon,
  DocumentTextIcon,
} from "@heroicons/react/24/outline";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import type { TreeNode } from "@/app/lib/knowhow/types";

type DocTreeProps = {
  tree: TreeNode[];
  activePath: string;
};

function DocTree({ tree, activePath }: DocTreeProps) {
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

  return (
    <ScrollArea className="h-full">
      <div className="space-y-0.5 py-1">
        {tree.map((node) => (
          <TreeNodeItem
            key={node.path}
            node={node}
            depth={0}
            activePath={activePath}
            expanded={expanded}
            onToggle={toggleFolder}
          />
        ))}
      </div>
    </ScrollArea>
  );
}

function TreeNodeItem({
  node,
  depth,
  activePath,
  expanded,
  onToggle,
}: {
  node: TreeNode;
  depth: number;
  activePath: string;
  expanded: Set<string>;
  onToggle: (path: string) => void;
}) {
  const isFolder = node.type === "folder";
  const isExpanded = expanded.has(node.path);
  const isActive = !isFolder && node.path === activePath;

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
          className={itemClasses}
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
        >
          {itemContent}
        </button>
      ) : (
        <Link
          href={`/docs/${node.path}`}
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
            />
          ))}
        </>
      )}
    </>
  );
}

export { DocTree };
export type { DocTreeProps };
