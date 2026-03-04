import "server-only";
import { cache } from "react";
import { gql } from "./client";
import type { Vault, Document, DocumentSummary } from "./types";

export const getVaults = cache(async (): Promise<Vault[]> => {
  const data = await gql<{ vaults: Vault[] }>(`
    query {
      vaults {
        id
        name
        description
      }
    }
  `);

  return data.vaults;
});

export const getVaultDocuments = cache(
  async (vaultId: string): Promise<DocumentSummary[]> => {
    const data = await gql<{ vault: { documents: DocumentSummary[] } | null }>(
      `
    query ($id: ID!) {
      vault(id: $id) {
        documents {
          id
          vaultId
          path
          title
          labels
          docType
          createdAt
          updatedAt
        }
      }
    }
  `,
      { id: vaultId },
    );

    return data.vault?.documents ?? [];
  },
);

export type FolderRecord = {
  path: string;
};

export const getVaultFolders = cache(
  async (vaultId: string): Promise<FolderRecord[]> => {
    const data = await gql<{ vault: { folders: FolderRecord[] } | null }>(
      `
    query ($id: ID!) {
      vault(id: $id) {
        folders {
          path
        }
      }
    }
  `,
      { id: vaultId },
    );

    return data.vault?.folders ?? [];
  },
);

export async function getDocument(
  vaultId: string,
  path: string,
): Promise<Document | null> {
  const data = await gql<{ document: Document | null }>(
    `
    query ($vaultId: ID!, $path: String!) {
      document(vaultId: $vaultId, path: $path) {
        id
        vaultId
        path
        title
        content
        contentBody
        labels
        docType
        createdAt
        updatedAt
        wikiLinks {
          id
          fromDocId
          toDocId
          rawTarget
          resolved
        }
        backlinks {
          id
          fromDocId
          toDocId
          rawTarget
          resolved
        }
      }
    }
  `,
    { vaultId, path },
  );

  return data.document;
}
