import "server-only";
import { env } from "@/app/lib/env";

type GraphQLResponse<T> = {
  data: T | null;
  errors?: { message: string }[];
};

export async function gql<T>(
  query: string,
  variables?: Record<string, unknown>,
): Promise<T> {
  const response = await fetch(env.KNOWHOW_API_URL, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${env.KNOWHOW_API_TOKEN}`,
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
