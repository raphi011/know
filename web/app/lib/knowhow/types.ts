export type Vault = {
  id: string;
  name: string;
  description: string | null;
};

export type DocumentSummary = {
  id: string;
  vaultId: string;
  path: string;
  title: string;
  labels: string[];
  docType: string | null;
  createdAt: string;
  updatedAt: string;
};

export type Document = DocumentSummary & {
  content: string;
  contentBody: string;
};

export type TreeNode =
  | { type: "folder"; name: string; path: string; children: TreeNode[] }
  | { type: "document"; name: string; path: string };

export type ChunkMatch = {
  snippet: string;
  headingPath: string | null;
};

export type SearchResult = {
  documentId: string;
  path: string;
  title: string;
  labels: string[];
  docType: string | null;
  score: number;
  matchedChunks: ChunkMatch[];
};

export type ServerConnection = {
  id: string;
  name: string;
  url: string;
  token: string;
};

const GRAPHQL_PATH = "/query";

/** Build the full GraphQL endpoint URL from a base server URL. */
export function graphqlUrl(baseUrl: string): string {
  return `${baseUrl}${GRAPHQL_PATH}`;
}

/** Strip the GraphQL path suffix from a URL (for normalizing user input). */
export function stripGraphqlPath(url: string): string {
  const trimmed = url.replace(/\/+$/, "");
  return trimmed.endsWith(GRAPHQL_PATH)
    ? trimmed.slice(0, -GRAPHQL_PATH.length)
    : trimmed;
}
