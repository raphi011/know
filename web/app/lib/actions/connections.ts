"use server";

import { revalidatePath } from "next/cache";
import { cookies } from "next/headers";
import { getServers } from "@/app/lib/env";
import type { ActionResult } from "@/app/lib/action-result";
import type { ServerConnection } from "@/app/lib/knowhow/types";

const ACTIVE_CONNECTION_COOKIE = "active_connection_id";
const ACTIVE_VAULT_COOKIE = "active_vault_id";
const COOKIE_MAX_AGE = 365 * 24 * 60 * 60; // 1 year

export async function getConnections(): Promise<ServerConnection[]> {
  return getServers();
}

export async function setActiveConnectionAction(
  connectionId: string,
): Promise<ActionResult> {
  const cookieStore = await cookies();
  cookieStore.set(ACTIVE_CONNECTION_COOKIE, connectionId, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    maxAge: COOKIE_MAX_AGE,
    path: "/",
  });
  // Clear vault selection when switching servers
  cookieStore.delete(ACTIVE_VAULT_COOKIE);

  revalidatePath("/", "layout");
  return { success: true };
}

export async function setActiveVaultAction(
  vaultId: string,
): Promise<ActionResult> {
  const cookieStore = await cookies();
  cookieStore.set(ACTIVE_VAULT_COOKIE, vaultId, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    maxAge: COOKIE_MAX_AGE,
    path: "/",
  });

  revalidatePath("/", "layout");
  return { success: true };
}

export async function getActiveConnectionId(): Promise<string | null> {
  const cookieStore = await cookies();
  return cookieStore.get(ACTIVE_CONNECTION_COOKIE)?.value ?? null;
}

export async function getActiveVaultId(): Promise<string | null> {
  const cookieStore = await cookies();
  return cookieStore.get(ACTIVE_VAULT_COOKIE)?.value ?? null;
}

/** Returns the active server connection, falling back to the first configured server. */
export async function getActiveConnection(): Promise<ServerConnection | null> {
  const servers = getServers();
  if (servers.length === 0) return null;

  const activeId = await getActiveConnectionId();
  if (activeId) {
    const match = servers.find((s) => s.id === activeId);
    if (match) return match;
  }

  return servers[0] ?? null;
}
