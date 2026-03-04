import "server-only";
import { getActiveConnection } from "@/app/lib/actions/connections";
import { graphqlUrl } from "@/app/lib/knowhow/types";
import { env } from "@/app/lib/env";

type GraphQLResponse<T> = {
  data: T | null;
  errors?: { message: string }[];
};

/**
 * Execute a GraphQL query against the active Knowhow server connection.
 * In auth mode, reads connection from the encrypted session cookie.
 * In no-auth mode (AUTH_DISABLED=true), uses BACKEND_URL directly.
 */
export async function gql<T>(
  query: string,
  variables?: Record<string, unknown>,
): Promise<T> {
  let url: string;
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };

  if (env.AUTH_DISABLED) {
    url = graphqlUrl(env.BACKEND_URL);
  } else {
    const conn = await getActiveConnection();
    if (!conn) {
      throw new Error(
        "No Knowhow server configured. Log in at /login to connect to a server.",
      );
    }
    url = graphqlUrl(conn.url);
    headers["Authorization"] = `Bearer ${conn.token}`;
  }

  const response = await fetch(url, {
    method: "POST",
    headers,
    body: JSON.stringify({ query, variables }),
    next: { revalidate: 0 }, // Disable Next.js fetch cache — all queries are dynamic
  });

  if (!response.ok) {
    const bodyText = await response.text().catch(() => "(unreadable body)");
    throw new Error(
      `Knowhow API HTTP ${response.status}: ${bodyText.slice(0, 200)}`,
    );
  }

  let json: GraphQLResponse<T>;
  try {
    json = (await response.json()) as GraphQLResponse<T>;
  } catch {
    throw new Error(
      `Knowhow API returned non-JSON response (status ${response.status})`,
    );
  }

  if (json.errors && json.errors.length > 0) {
    const messages = json.errors.map((e) => e.message).join("; ");
    throw new Error(`Knowhow API errors: ${messages}`);
  }

  if (!json.data) {
    throw new Error("GraphQL response contained no data");
  }

  return json.data;
}
