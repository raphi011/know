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
        { error: "No Knowhow server configured" },
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
      { error: "Failed to read request body" },
      { status: 400 },
    );
  }

  let upstream: Response;
  try {
    upstream = await fetch(`${backendUrl}/agent/chat`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...authHeader,
      },
      body,
    });
  } catch (err) {
    console.error("Agent API unreachable:", err);
    return NextResponse.json(
      { error: "Agent API is unreachable" },
      { status: 502 },
    );
  }

  if (!upstream.ok) {
    const text = await upstream.text().catch(() => "Unknown error");
    return NextResponse.json(
      { error: `Agent API error: ${text}` },
      { status: upstream.status },
    );
  }

  if (!upstream.body) {
    return NextResponse.json(
      { error: "No response body from agent" },
      { status: 502 },
    );
  }

  // Stream the SSE response back to the client
  const stream = new ReadableStream({
    async start(controller) {
      const reader = upstream.body!.getReader();
      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          controller.enqueue(value);
        }
      } catch (err) {
        console.error("Stream relay error:", err);
        const errorEvent = new TextEncoder().encode(
          `data: ${JSON.stringify({ type: "error", content: "Connection to agent lost" })}\n\n`,
        );
        controller.enqueue(errorEvent);
      } finally {
        controller.close();
      }
    },
  });

  return new Response(stream, {
    headers: {
      "Content-Type": "text/event-stream",
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "X-Accel-Buffering": "no",
    },
  });
}
