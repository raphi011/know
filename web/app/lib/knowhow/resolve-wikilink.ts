import type { DocumentSummary } from "./types";

type ResolvedLink = { path: string; title: string };

/** Strip at most one leading `/` and one trailing `.md` for comparison. */
function normalizePath(p: string): string {
  let s = p;
  if (s.startsWith("/")) s = s.slice(1);
  if (s.endsWith(".md")) s = s.slice(0, -3);
  return s;
}

/**
 * Resolve a wikilink target against a list of documents.
 *
 * Resolution order:
 * 1. Exact path match (with/without leading `/`, with/without `.md`)
 * 2. Path suffix match (`"go-patterns"` → `"notes/go-patterns.md"`)
 * 3. Case-insensitive title match
 */
export function resolveWikiLink(
  target: string,
  documents: DocumentSummary[],
): ResolvedLink | null {
  const normalized = normalizePath(target);

  // 1. Exact path match
  for (const doc of documents) {
    if (normalizePath(doc.path) === normalized) {
      return { path: doc.path, title: doc.title };
    }
  }

  // 2. Path suffix match — target matches the end of a document path
  const suffix = "/" + normalized;
  for (const doc of documents) {
    if (normalizePath(doc.path).endsWith(suffix)) {
      return { path: doc.path, title: doc.title };
    }
  }

  // 3. Case-insensitive title match
  const lowerTarget = target.toLowerCase();
  for (const doc of documents) {
    if (doc.title.toLowerCase() === lowerTarget) {
      return { path: doc.path, title: doc.title };
    }
  }

  return null;
}
