import "server-only";
import { getActiveConnection } from "@/app/lib/actions/connections";
import { graphqlUrl } from "@/app/lib/knowhow/types";

type GraphQLResponse<T> = {
  data: T | null;
  errors?: { message: string }[];
};

/**
 * Execute a GraphQL query against the active Knowhow server connection.
 * Server connections are stored in an encrypted session cookie.
 */
export async function gql<T>(
  query: string,
  variables?: Record<string, unknown>,
): Promise<T> {
  const conn = await getActiveConnection();
  if (!conn) {
    throw new Error(
      "No Knowhow server configured. Log in at /login to connect to a server.",
    );
  }

  const response = await fetch(graphqlUrl(conn.url), {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${conn.token}`,
    },
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
