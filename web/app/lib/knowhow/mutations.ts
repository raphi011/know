import type { ActionResult } from "@/app/lib/action-result";

export async function saveDocument(
  vaultId: string,
  path: string,
  content: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $path: String!, $content: String!) {
            updateDocument(vaultId: $vaultId, path: $path, content: $content) {
              id
            }
          }
        `,
        variables: { vaultId, path, content },
      }),
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
    console.error("Document save failed:", err);
    return { success: false, error: message };
  }
}

export async function createDocument(
  vaultId: string,
  path: string,
  content: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $file: FileInput!) {
            createDocument(vaultId: $vaultId, file: $file) { id }
          }
        `,
        variables: { vaultId, file: { path, content } },
      }),
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
    console.error("Document create failed:", err);
    return { success: false, error: message };
  }
}

export async function deleteDocument(
  vaultId: string,
  path: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $path: String!) {
            deleteDocument(vaultId: $vaultId, path: $path)
          }
        `,
        variables: { vaultId, path },
      }),
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
    console.error("Document delete failed:", err);
    return { success: false, error: message };
  }
}

export async function moveDocument(
  vaultId: string,
  oldPath: string,
  newPath: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $oldPath: String!, $newPath: String!) {
            moveDocument(vaultId: $vaultId, oldPath: $oldPath, newPath: $newPath) { id }
          }
        `,
        variables: { vaultId, oldPath, newPath },
      }),
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
    console.error("Document move failed:", err);
    return { success: false, error: message };
  }
}

export async function deleteDocumentsByPrefix(
  vaultId: string,
  pathPrefix: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $pathPrefix: String!) {
            deleteDocumentsByPrefix(vaultId: $vaultId, pathPrefix: $pathPrefix)
          }
        `,
        variables: { vaultId, pathPrefix },
      }),
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
    console.error("Documents delete by prefix failed:", err);
    return { success: false, error: message };
  }
}

export async function moveDocumentsByPrefix(
  vaultId: string,
  oldPrefix: string,
  newPrefix: string,
): Promise<ActionResult> {
  try {
    const response = await fetch("/api/graphql", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        query: `
          mutation ($vaultId: ID!, $oldPrefix: String!, $newPrefix: String!) {
            moveDocumentsByPrefix(vaultId: $vaultId, oldPrefix: $oldPrefix, newPrefix: $newPrefix)
          }
        `,
        variables: { vaultId, oldPrefix, newPrefix },
      }),
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
    console.error("Documents move by prefix failed:", err);
    return { success: false, error: message };
  }
}
