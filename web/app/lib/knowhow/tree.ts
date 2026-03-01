import type { DocumentSummary, TreeNode } from "./types";

export function buildTree(documents: DocumentSummary[]): TreeNode[] {
  const root: TreeNode[] = [];
  const folderMap = new Map<string, TreeNode & { type: "folder" }>();

  function ensureFolder(folderPath: string): TreeNode & { type: "folder" } {
    const existing = folderMap.get(folderPath);
    if (existing) return existing;

    const parts = folderPath.split("/");
    const name = parts.at(-1)!;

    const node: TreeNode & { type: "folder" } = {
      name,
      path: folderPath,
      type: "folder",
      children: [],
    };
    folderMap.set(folderPath, node);

    if (parts.length === 1) {
      root.push(node);
    } else {
      const parentPath = parts.slice(0, -1).join("/");
      const parent = ensureFolder(parentPath);
      parent.children.push(node);
    }

    return node;
  }

  for (const doc of documents) {
    const normalizedPath = doc.path.replace(/^\//, "");
    const parts = normalizedPath.split("/");
    const name = parts.at(-1)!;

    const docNode: TreeNode = {
      name: name.replace(/\.md$/, ""),
      path: normalizedPath,
      type: "document",
    };

    if (parts.length === 1) {
      root.push(docNode);
    } else {
      const folderPath = parts.slice(0, -1).join("/");
      const parent = ensureFolder(folderPath);
      parent.children.push(docNode);
    }
  }

  sortTree(root);
  return root;
}

function sortTree(nodes: TreeNode[]) {
  nodes.sort((a, b) => {
    if (a.type !== b.type) return a.type === "folder" ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  for (const node of nodes) {
    if (node.type === "folder") {
      sortTree(node.children);
    }
  }
}
