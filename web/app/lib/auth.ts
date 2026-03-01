import "server-only";
import NextAuth from "next-auth";
import { DrizzleAdapter } from "@auth/drizzle-adapter";
import { db } from "@/app/lib/db";
import { cookies } from "next/headers";
import { env } from "@/app/lib/env";
import type { Theme } from "@/app/lib/types";
import type { Locale } from "@/app/lib/types";

const THEME_COOKIE_NAME = "theme";
const LOCALE_COOKIE_NAME = "locale";
const PREF_COOKIE_MAX_AGE = 365 * 24 * 60 * 60; // 1 year

export const { handlers, signIn, signOut, auth } = NextAuth({
  adapter: DrizzleAdapter(db),
  providers: [
    {
      id: "oidc",
      name: "SSO",
      type: "oidc",
      issuer: env.AUTH_OIDC_ISSUER,
      clientId: env.AUTH_OIDC_CLIENT_ID,
      clientSecret: env.AUTH_OIDC_CLIENT_SECRET,
    },
  ],
  pages: {
    signIn: "/login",
  },
  callbacks: {
    session({ session, user }) {
      session.user.id = user.id;
      return session;
    },
  },
});

export async function getCurrentUser() {
  const session = await auth();
  return session?.user ?? null;
}

export async function getSession() {
  return auth();
}

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
    httpOnly: false, // readable by client-side locale detection if needed
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
