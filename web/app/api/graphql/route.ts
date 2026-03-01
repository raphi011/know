import { NextRequest, NextResponse } from "next/server";
import { getSession } from "@/app/lib/session";
import { getActiveConnection } from "@/app/lib/actions/connections";

export async function POST(request: NextRequest) {
  const session = await getSession();
  if (!session || session.servers.length === 0) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
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

  const conn = await getActiveConnection();
  if (!conn) {
    return NextResponse.json(
      { errors: [{ message: "No Knowhow server configured" }] },
      { status: 503 },
    );
  }

  let response: Response;
  try {
    response = await fetch(conn.url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${conn.token}`,
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
