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
