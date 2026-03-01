"use server";

import { revalidatePath } from "next/cache";
import { cookies } from "next/headers";
import { getSession } from "@/app/lib/session";
import type { ServerConnection } from "@/app/lib/knowhow/types";

const ACTIVE_CONNECTION_COOKIE = "active_connection_id";
const ACTIVE_VAULT_COOKIE = "active_vault_id";
const COOKIE_MAX_AGE = 365 * 24 * 60 * 60; // 1 year

export async function getConnections(): Promise<ServerConnection[]> {
  const session = await getSession();
  return session?.servers ?? [];
}

export async function setActiveConnectionAction(
  connectionId: string,
): Promise<void> {
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
}

export async function setActiveVaultAction(vaultId: string): Promise<void> {
  const cookieStore = await cookies();
  cookieStore.set(ACTIVE_VAULT_COOKIE, vaultId, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    maxAge: COOKIE_MAX_AGE,
    path: "/",
  });

  revalidatePath("/", "layout");
}

export async function getActiveConnectionId(): Promise<string | null> {
  const cookieStore = await cookies();
  return cookieStore.get(ACTIVE_CONNECTION_COOKIE)?.value ?? null;
}

export async function getActiveVaultId(): Promise<string | null> {
  const cookieStore = await cookies();
  return cookieStore.get(ACTIVE_VAULT_COOKIE)?.value ?? null;
}

/** Returns the active server connection from the session cookie. */
export async function getActiveConnection(): Promise<ServerConnection | null> {
  const servers = await getConnections();
  if (servers.length === 0) return null;

  const activeId = await getActiveConnectionId();
  if (activeId) {
    const match = servers.find((s) => s.id === activeId);
    if (match) return match;
  }

  return servers[0] ?? null;
}
