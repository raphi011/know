import "server-only";

import { cookies } from "next/headers";
import { env } from "./env";
import type { ServerConnection } from "@/app/lib/knowhow/types";

const SESSION_COOKIE = "kh_session";
const COOKIE_MAX_AGE = 365 * 24 * 60 * 60; // 1 year

export type Session = {
  /** Servers the user has connected to. */
  servers: ServerConnection[];
};

// ── Encryption helpers (AES-256-GCM via Web Crypto) ─────────

async function deriveKey(secret: string): Promise<CryptoKey> {
  const keyMaterial = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    "PBKDF2",
    false,
    ["deriveKey"],
  );
  const salt = new TextEncoder().encode("knowhow-session-v1");
  return crypto.subtle.deriveKey(
    { name: "PBKDF2", salt, iterations: 100_000, hash: "SHA-256" },
    keyMaterial,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"],
  );
}

// Eagerly derive at module load so the first request is fast.
const keyPromise = deriveKey(
  env.SESSION_SECRET || "dev-secret-not-for-production-use-only",
);

async function encrypt(data: string): Promise<string> {
  console.time("session:encrypt");
  const key = await keyPromise;
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const encrypted = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv },
    key,
    new TextEncoder().encode(data),
  );
  // Encode as base64: iv (12 bytes) + ciphertext
  const combined = new Uint8Array(iv.length + encrypted.byteLength);
  combined.set(iv);
  combined.set(new Uint8Array(encrypted), iv.length);
  console.timeEnd("session:encrypt");
  return btoa(String.fromCharCode(...combined));
}

async function decrypt(encoded: string): Promise<string> {
  console.time("session:decrypt");
  const key = await keyPromise;
  const combined = Uint8Array.from(atob(encoded), (c) => c.charCodeAt(0));
  const iv = combined.slice(0, 12);
  const ciphertext = combined.slice(12);
  const decrypted = await crypto.subtle.decrypt(
    { name: "AES-GCM", iv },
    key,
    ciphertext,
  );
  console.timeEnd("session:decrypt");
  return new TextDecoder().decode(decrypted);
}

// ── Session CRUD ────────────────────────────────────

export async function getSession(): Promise<Session | null> {
  const cookieStore = await cookies();
  const raw = cookieStore.get(SESSION_COOKIE)?.value;
  if (!raw) return null;

  try {
    const json = await decrypt(raw);
    return JSON.parse(json) as Session;
  } catch {
    // Corrupt or tampered cookie — treat as logged out
    return null;
  }
}

export async function setSession(session: Session): Promise<void> {
  const cookieStore = await cookies();
  const encrypted = await encrypt(JSON.stringify(session));
  cookieStore.set(SESSION_COOKIE, encrypted, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    maxAge: COOKIE_MAX_AGE,
    path: "/",
  });
}

export async function clearSession(): Promise<void> {
  const cookieStore = await cookies();
  cookieStore.delete(SESSION_COOKIE);
}

/** Checks whether a valid session cookie exists (no decryption — fast). */
export async function hasSession(): Promise<boolean> {
  const cookieStore = await cookies();
  return cookieStore.has(SESSION_COOKIE);
}
