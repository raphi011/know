import "server-only";
import { cookies } from "next/headers";
import type { Theme } from "@/app/lib/types";
import type { Locale } from "@/app/lib/types";

const THEME_COOKIE_NAME = "theme";
const LOCALE_COOKIE_NAME = "locale";
const PREF_COOKIE_MAX_AGE = 365 * 24 * 60 * 60; // 1 year

// ── Theme cookie helpers ────────────────────────────

export async function setThemeCookie(theme: Theme): Promise<void> {
  const cookieStore = await cookies();
  cookieStore.set(THEME_COOKIE_NAME, theme, {
    httpOnly: false, // readable by inline <script> for FOUC prevention
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    maxAge: PREF_COOKIE_MAX_AGE,
    path: "/",
  });
}

export async function getThemeCookie(): Promise<Theme> {
  const cookieStore = await cookies();
  const value = cookieStore.get(THEME_COOKIE_NAME)?.value;
  if (value === "light" || value === "dark") return value;
  return "auto";
}

// ── Locale cookie helpers ───────────────────────────

export async function setLocaleCookie(locale: Locale): Promise<void> {
  const cookieStore = await cookies();
  cookieStore.set(LOCALE_COOKIE_NAME, locale, {
    httpOnly: false,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    maxAge: PREF_COOKIE_MAX_AGE,
    path: "/",
  });
}

export async function getLocaleCookie(): Promise<Locale> {
  const cookieStore = await cookies();
  const value = cookieStore.get(LOCALE_COOKIE_NAME)?.value;
  if (value === "en") return "en";
  return "de";
}
