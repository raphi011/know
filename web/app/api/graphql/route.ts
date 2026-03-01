import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/app/lib/auth";
import { env } from "@/app/lib/env";
import { getActiveConnection } from "@/app/lib/actions/connections";

export async function POST(request: NextRequest) {
  // Set DISABLE_AUTH=1 to skip during local UI testing without OIDC
  if (process.env.DISABLE_AUTH !== "1") {
    const session = await auth();
    if (!session?.user) {
      return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    }
  }

  let body: string;
  try {
    body = await request.text();
  } catch {
    return NextResponse.json(
      { errors: [{ message: "Failed to read request body" }] },
      { status: 400 },
    );
  }

  // Use active server connection, fall back to env vars
  const conn = await getActiveConnection();
  const apiUrl = conn?.url ?? env.KNOWHOW_API_URL;
  const apiToken = conn?.apiToken ?? env.KNOWHOW_API_TOKEN;

  let response: Response;
  try {
    response = await fetch(apiUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${apiToken}`,
      },
      body,
    });
  } catch (err) {
    console.error("Knowhow API unreachable:", err);
    return NextResponse.json(
      { errors: [{ message: "Knowhow API is unreachable" }] },
      { status: 502 },
    );
  }

  let data: unknown;
  try {
    data = await response.json();
  } catch {
    console.error(
      `Knowhow API returned non-JSON response (status ${response.status})`,
    );
    return NextResponse.json(
      { errors: [{ message: `Upstream error (HTTP ${response.status})` }] },
      { status: 502 },
    );
  }

  return NextResponse.json(data, { status: response.status });
}
