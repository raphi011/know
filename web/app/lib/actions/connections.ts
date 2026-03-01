"use server";

import { revalidatePath } from "next/cache";
import { cookies } from "next/headers";
import { db } from "@/app/lib/db";
import { serverConnections } from "@/app/lib/db/schema";
import { eq } from "drizzle-orm";
import type { ActionResult } from "@/app/lib/action-result";

const ACTIVE_CONNECTION_COOKIE = "active_connection_id";
const ACTIVE_VAULT_COOKIE = "active_vault_id";
const COOKIE_MAX_AGE = 365 * 24 * 60 * 60; // 1 year

export type ServerConnection = {
  id: string;
  name: string;
  url: string;
  apiToken: string;
  isDefault: boolean;
  createdAt: Date;
};

export async function getConnections(): Promise<ServerConnection[]> {
  return db
    .select()
    .from(serverConnections)
    .orderBy(serverConnections.createdAt);
}

export async function addConnectionAction(
  name: string,
  url: string,
  apiToken: string,
): Promise<ActionResult & { id?: string }> {
  if (!name.trim() || !url.trim() || !apiToken.trim()) {
    return { success: false, error: "All fields are required" };
  }

  try {
    const existing = await db.select().from(serverConnections);
    const isDefault = existing.length === 0;

    const [row] = await db
      .insert(serverConnections)
      .values({
        name: name.trim(),
        url: url.trim().replace(/\/$/, ""),
        apiToken: apiToken.trim(),
        isDefault,
      })
      .returning({ id: serverConnections.id });

    if (!row) {
      return { success: false, error: "Failed to create connection" };
    }

    // If this is the first connection, set it as active
    if (isDefault) {
      const cookieStore = await cookies();
      cookieStore.set(ACTIVE_CONNECTION_COOKIE, row.id, {
        httpOnly: true,
        secure: process.env.NODE_ENV === "production",
        sameSite: "lax",
        maxAge: COOKIE_MAX_AGE,
        path: "/",
      });
    }

    revalidatePath("/", "layout");
    return { success: true, id: row.id };
  } catch (error) {
    console.error("Failed to add connection:", error);
    return { success: false, error: "Failed to add connection" };
  }
}

export async function removeConnectionAction(
  id: string,
): Promise<ActionResult> {
  try {
    await db
      .delete(serverConnections)
      .where(eq(serverConnections.id, id));

    // If we deleted the active connection, clear the cookie
    const cookieStore = await cookies();
    const activeId = cookieStore.get(ACTIVE_CONNECTION_COOKIE)?.value;
    if (activeId === id) {
      // Pick the next available connection
      const remaining = await db.select().from(serverConnections).limit(1);
      if (remaining.length > 0) {
        cookieStore.set(ACTIVE_CONNECTION_COOKIE, remaining[0]!.id, {
          httpOnly: true,
          secure: process.env.NODE_ENV === "production",
          sameSite: "lax",
          maxAge: COOKIE_MAX_AGE,
          path: "/",
        });
      } else {
        cookieStore.delete(ACTIVE_CONNECTION_COOKIE);
      }
      cookieStore.delete(ACTIVE_VAULT_COOKIE);
    }

    revalidatePath("/", "layout");
    return { success: true };
  } catch (error) {
    console.error("Failed to remove connection:", error);
    return { success: false, error: "Failed to remove connection" };
  }
}

export async function setActiveConnectionAction(
  connectionId: string,
): Promise<ActionResult> {
  try {
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
  } catch (error) {
    console.error("Failed to set active connection:", error);
    return { success: false, error: "Failed to switch connection" };
  }
}

export async function setActiveVaultAction(
  vaultId: string,
): Promise<ActionResult> {
  try {
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
  } catch (error) {
    console.error("Failed to set active vault:", error);
    return { success: false, error: "Failed to switch vault" };
  }
}

export async function getActiveConnectionId(): Promise<string | null> {
  const cookieStore = await cookies();
  return cookieStore.get(ACTIVE_CONNECTION_COOKIE)?.value ?? null;
}

export async function getActiveVaultId(): Promise<string | null> {
  const cookieStore = await cookies();
  return cookieStore.get(ACTIVE_VAULT_COOKIE)?.value ?? null;
}

/** Returns the active connection or the default, or null if none configured. */
export async function getActiveConnection(): Promise<ServerConnection | null> {
  const activeId = await getActiveConnectionId();

  if (activeId) {
    const [conn] = await db
      .select()
      .from(serverConnections)
      .where(eq(serverConnections.id, activeId))
      .limit(1);
    if (conn) return conn;
  }

  // Fall back to default connection
  const [defaultConn] = await db
    .select()
    .from(serverConnections)
    .where(eq(serverConnections.isDefault, true))
    .limit(1);
  if (defaultConn) return defaultConn;

  // Fall back to first connection
  const [first] = await db
    .select()
    .from(serverConnections)
    .limit(1);
  return first ?? null;
}
