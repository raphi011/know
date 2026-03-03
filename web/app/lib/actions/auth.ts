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
  console.time("loginAction:total");
  const url = stripGraphqlPath(serverUrl.trim());
  const token = serverToken.trim();
  const name = serverName.trim() || new URL(url).hostname;

  if (!url || !token) {
    console.timeEnd("loginAction:total");
    return { success: false, error: "Server URL and token are required" };
  }

  // Validate the connection by making a test query
  try {
    console.time("loginAction:fetch");
    const response = await fetch(graphqlUrl(url), {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ query: "{ vaults { id name } }" }),
    });
    console.timeEnd("loginAction:fetch");

    if (response.status === 401) {
      console.timeEnd("loginAction:total");
      return { success: false, error: "Invalid API token" };
    }

    if (!response.ok) {
      console.timeEnd("loginAction:total");
      return {
        success: false,
        error: `Server returned HTTP ${response.status}`,
      };
    }
  } catch {
    console.timeEnd("loginAction:total");
    return { success: false, error: "Cannot reach server" };
  }

  // Add to session (or create new session)
  console.time("loginAction:getSession");
  const session = (await getSession()) ?? { servers: [] };
  console.timeEnd("loginAction:getSession");
  const id = name.toLowerCase().replace(/\s+/g, "-");

  // Replace if same id exists, otherwise append
  const existing = session.servers.findIndex((s) => s.id === id);
  const conn: ServerConnection = { id, name, url, token };
  if (existing >= 0) {
    session.servers[existing] = conn;
  } else {
    session.servers.push(conn);
  }

  console.time("loginAction:setSession");
  await setSession(session);
  console.timeEnd("loginAction:setSession");
  console.timeEnd("loginAction:total");
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
