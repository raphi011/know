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
