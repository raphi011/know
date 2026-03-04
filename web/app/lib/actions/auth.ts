"use server";

import { redirect } from "next/navigation";
import { getSession, setSession, clearSession } from "@/app/lib/session";
import {
  type ServerConnection,
  graphqlUrl,
  stripGraphqlPath,
} from "@/app/lib/knowhow/types";

export async function loginAction(
  serverUrl: string,
  serverToken: string,
  serverName: string,
): Promise<{ success: boolean; error?: string }> {
  const url = stripGraphqlPath(serverUrl.trim());
  const token = serverToken.trim();

  let name: string;
  try {
    name = serverName.trim() || new URL(url).hostname;
  } catch {
    return { success: false, error: "Invalid server URL" };
  }

  if (!url || !token) {
    return { success: false, error: "Server URL and token are required" };
  }

  // Validate the connection by making a test query
  try {
    const response = await fetch(graphqlUrl(url), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ query: "{ vaults { id name } }" }),
    });

    if (response.status === 401) {
      return { success: false, error: "Invalid API token" };
    }

    if (!response.ok) {
      return {
        success: false,
        error: `Server returned HTTP ${response.status}`,
      };
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    return { success: false, error: `Cannot reach server: ${message}` };
  }

  // Add to session (or create new session)
  const session = (await getSession()) ?? { servers: [] };
  const id = name.toLowerCase().replace(/\s+/g, "-");

  // Replace if same id exists, otherwise append
  const existing = session.servers.findIndex((s) => s.id === id);
  const conn: ServerConnection = { id, name, url, token };
  if (existing >= 0) {
    session.servers[existing] = conn;
  } else {
    session.servers.push(conn);
  }

  await setSession(session);
  return { success: true };
}

export async function logoutAction(): Promise<void> {
  await clearSession();
  redirect("/login");
}

export async function removeServerAction(serverId: string): Promise<void> {
  const session = await getSession();
  if (!session) return;

  session.servers = session.servers.filter((s) => s.id !== serverId);

  if (session.servers.length === 0) {
    await clearSession();
    redirect("/login");
  } else {
    await setSession(session);
  }
}
