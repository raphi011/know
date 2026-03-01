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
