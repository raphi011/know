import type { DocumentSummary } from "./types";

/** Get the basename (last segment) of a path */
function basename(path: string): string {
  const parts = path.split("/");
  return parts[parts.length - 1] ?? "";
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
