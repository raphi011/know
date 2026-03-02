"use client";

import { usePathname } from "next/navigation";
import { DocTree } from "@/components/doc-tree";
import type { TreeNode } from "@/app/lib/knowhow/types";

type DocSidebarProps = {
  tree: TreeNode[];
  vaultId: string;
};

function DocSidebar({ tree, vaultId }: DocSidebarProps) {
  const pathname = usePathname();

  // Extract document path from URL: /docs/foo/bar.md → foo/bar.md
  const activePath = pathname.startsWith("/docs/")
    ? pathname.slice("/docs/".length)
    : "";

  return <DocTree tree={tree} activePath={activePath} vaultId={vaultId} />;
}

export { DocSidebar };
