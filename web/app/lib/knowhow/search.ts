import { slug } from "github-slugger";
import type { SearchResult } from "@/app/lib/knowhow/types";

export async function searchDocuments(
  vaultId: string,
  query: string,
  limit = 10,
  signal?: AbortSignal,
): Promise<SearchResult[]> {
  const response = await fetch("/api/graphql", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    signal,
    body: JSON.stringify({
      query: `
        query ($input: SearchInput!) {
          search(input: $input) {
            documentId
            path
            title
            labels
            docType
            score
            matchedChunks {
              snippet
              headingPath
            }
          }
        }
      `,
      variables: { input: { vaultId, query, limit } },
    }),
  });

  if (!response.ok) {
    const bodyText = await response.text().catch(() => "");
    throw new Error(
      bodyText
        ? `Search failed (HTTP ${response.status}): ${bodyText.slice(0, 200)}`
        : `Search failed (HTTP ${response.status})`,
    );
  }

  let json: {
    data?: { search: SearchResult[] };
    errors?: { message: string }[];
  };
  try {
    json = await response.json();
  } catch (err) {
    throw new Error("Server returned an invalid response", { cause: err });
  }

  if (json.errors?.length) {
    throw new Error(json.errors[0]!.message);
  }

  if (!json.data?.search) {
    throw new Error("Server returned an unexpected response shape");
  }

  return json.data.search;
}

/**
 * Format a headingPath for display: "## Setup > ### Install" → "Setup › Install".
 */
export function formatHeadingPath(headingPath: string): string {
  return headingPath.replace(/^#+\s*/g, "").replaceAll(/\s*>\s*#+\s*/g, " › ");
}

/**
 * Extract a URL hash fragment from a headingPath like "## Setup > ### Install".
 * Returns the slug of the deepest heading, or empty string if none.
 */
export function headingPathToHash(headingPath: string | null): string {
  if (!headingPath) return "";
  // Take the last segment: "## Setup > ### Install" → "### Install"
  const last = headingPath.split(" > ").pop() ?? "";
  // Strip the markdown heading prefix: "### Install" → "Install"
  const heading = last.replace(/^#+\s*/, "");
  if (!heading) return "";
  return `#${slug(heading)}`;
}
