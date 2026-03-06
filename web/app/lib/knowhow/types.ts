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

export type WikiLink = {
  id: string;
  fromDocId: string;
  rawTarget: string;
} & (
  | { resolved: true; toDocId: string }
  | { resolved: false; toDocId: null }
);

export type Document = DocumentSummary & {
  content: string;
  contentBody: string;
  wikiLinks: WikiLink[];
  backlinks: WikiLink[];
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

export type DocumentVersion = {
  id: string;
  documentId: string;
  vaultId: string;
  version: number;
  title: string;
  contentHash: string;
  source: string;
  createdAt: string;
};

export type DocumentVersionConnection = {
  versions: DocumentVersion[];
  totalCount: number;
};

export type DiffLine = {
  type: "CONTEXT" | "ADD" | "DELETE";
  content: string;
  oldLineNo: number | null;
  newLineNo: number | null;
};

export type DiffHunk = {
  index: number;
  oldStart: number;
  oldLines: number;
  newStart: number;
  newLines: number;
  header: string;
  lines: DiffLine[];
};

export type DiffStats = {
  additions: number;
  deletions: number;
  hunksCount: number;
};

export type VersionDiff = {
  hunks: DiffHunk[];
  hasConflict: boolean;
  stats: DiffStats;
};

export type ServerConfig = {
  llmProvider: string;
  llmModel: string;
  embedProvider: string;
  embedModel: string;
  embedDimension: number;
  semanticSearchEnabled: boolean;
  agentChatEnabled: boolean;
  webSearchEnabled: boolean;
  chunkThreshold: number;
  chunkTargetSize: number;
  chunkMaxSize: number;
  versionCoalesceMinutes: number;
  versionRetentionCount: number;
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
