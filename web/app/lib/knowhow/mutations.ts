import type { ActionResult } from "@/app/lib/action-result";

async function graphqlMutation(
  query: string,
  variables: Record<string, unknown>,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query, variables }),
    });

    if (!response.ok) {
      return { success: false, error: `HTTP ${response.status}` };
    }

    let json: { errors?: { message: string }[] };
    try {
      json = await response.json();
    } catch {
      return { success: false, error: "Server returned an invalid response" };
    }

    if (json.errors?.length) {
      return { success: false, error: json.errors[0]!.message };
    }

    return { success: true };
  } catch (err) {
    const message = err instanceof Error ? err.message : "Unknown error";
    return { success: false, error: message };
  }
}

export function saveDocument(vaultId: string, path: string, content: string) {
  return graphqlMutation(
    `mutation ($vaultId: ID!, $path: String!, $content: String!) {
      updateDocument(vaultId: $vaultId, path: $path, content: $content) { id }
    }`,
    { vaultId, path, content },
  );
}

export function createDocument(vaultId: string, path: string, content: string) {
  return graphqlMutation(
    `mutation ($vaultId: ID!, $file: FileInput!) {
      createDocument(vaultId: $vaultId, file: $file) { id }
    }`,
    { vaultId, file: { path, content } },
  );
}

export function deleteDocument(vaultId: string, path: string) {
  return graphqlMutation(
    `mutation ($vaultId: ID!, $path: String!) {
      deleteDocument(vaultId: $vaultId, path: $path)
    }`,
    { vaultId, path },
  );
}

export function moveDocument(
  vaultId: string,
  oldPath: string,
  newPath: string,
) {
  return graphqlMutation(
    `mutation ($vaultId: ID!, $oldPath: String!, $newPath: String!) {
      moveDocument(vaultId: $vaultId, oldPath: $oldPath, newPath: $newPath) { id }
    }`,
    { vaultId, oldPath, newPath },
  );
}

export function deleteDocumentsByPrefix(vaultId: string, pathPrefix: string) {
  return graphqlMutation(
    `mutation ($vaultId: ID!, $pathPrefix: String!) {
      deleteDocumentsByPrefix(vaultId: $vaultId, pathPrefix: $pathPrefix)
    }`,
    { vaultId, pathPrefix },
  );
}

export function moveDocumentsByPrefix(
  vaultId: string,
  oldPrefix: string,
  newPrefix: string,
) {
  return graphqlMutation(
    `mutation ($vaultId: ID!, $oldPrefix: String!, $newPrefix: String!) {
      moveDocumentsByPrefix(vaultId: $vaultId, oldPrefix: $oldPrefix, newPrefix: $newPrefix)
    }`,
    { vaultId, oldPrefix, newPrefix },
  );
}
