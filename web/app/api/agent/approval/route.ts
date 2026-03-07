import { NextRequest, NextResponse } from "next/server";
import { getSession } from "@/app/lib/session";
import { getActiveConnection } from "@/app/lib/actions/connections";
import { env } from "@/app/lib/env";

export async function POST(request: NextRequest) {
  let backendUrl: string;
  let authHeader: Record<string, string> = {};

  if (env.AUTH_DISABLED) {
    backendUrl = env.BACKEND_URL;
  } else {
    const session = await getSession();
    if (!session || session.servers.length === 0) {
      return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    }

    const conn = await getActiveConnection();
    if (!conn) {
      return NextResponse.json(
        { error: "No server configured" },
        { status: 503 },
      );
    }
    backendUrl = conn.url;
    authHeader = { Authorization: `Bearer ${conn.token}` };
  }

  let body: string;
  try {
    body = await request.text();
  } catch {
    return NextResponse.json(
      { error: "Failed to read body" },
      { status: 400 },
    );
  }

  try {
    const res = await fetch(`${backendUrl}/agent/approval`, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authHeader },
      body,
    });
    if (!res.ok) {
      const text = await res.text().catch(() => "Unknown error");
      return NextResponse.json({ error: text }, { status: res.status });
    }
    return NextResponse.json({ ok: true });
  } catch (err) {
    console.error("Agent approval API unreachable:", err);
    return NextResponse.json(
      { error: "Agent API unreachable" },
      { status: 502 },
    );
  }
}
