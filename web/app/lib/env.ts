import "server-only";

import type { ServerConnection } from "@/app/lib/knowhow/types";

// During `next build`, env vars like DATABASE_URL aren't available.
// Skip validation so static page generation can complete.
const isBuildPhase = process.env.NEXT_PHASE === "phase-production-build";

function required(name: string): string {
  const value = process.env[name];
  if (!value) {
    if (isBuildPhase) return "";
    throw new Error(`Missing required environment variable: ${name}`);
  }
  return value;
}

function requiredInProduction(name: string): string {
  const value = process.env[name];
  if (!value && process.env.NODE_ENV === "production" && !isBuildPhase) {
    throw new Error(
      `Missing environment variable: ${name} (required in production)`,
    );
  }
  if (!value) {
    if (!isBuildPhase) {
      console.warn(`[env] ${name} is not set (required in production)`);
    }
  }
  return value ?? "";
}

/**
 * Parse server connections from environment variables.
 *
 * Format: KNOWHOW_SERVER_<NAME>_URL + KNOWHOW_SERVER_<NAME>_TOKEN
 * Example:
 *   KNOWHOW_SERVER_WORK_URL=http://work:8484/query
 *   KNOWHOW_SERVER_WORK_TOKEN=kh_abc123
 *   KNOWHOW_SERVER_PRIVATE_URL=http://home:8484/query
 *   KNOWHOW_SERVER_PRIVATE_TOKEN=kh_xyz789
 *
 * Each server needs a URL and TOKEN pair.
 */
function parseServers(): ServerConnection[] {
  const servers: ServerConnection[] = [];
  const seen = new Set<string>();

  // Scan for KNOWHOW_SERVER_*_URL env vars
  for (const key of Object.keys(process.env)) {
    const match = key.match(/^KNOWHOW_SERVER_(.+)_URL$/);
    if (!match) continue;

    const name = match[1]!;
    if (seen.has(name)) continue;
    seen.add(name);

    const url = process.env[key];
    const token = process.env[`KNOWHOW_SERVER_${name}_TOKEN`];
    if (!url) continue;

    servers.push({
      id: name.toLowerCase(),
      name: name.charAt(0) + name.slice(1).toLowerCase(),
      url,
      token: token ?? "",
    });
  }

  return servers;
}

let _servers: ServerConnection[] | null = null;

export function getServers(): ServerConnection[] {
  if (!_servers) {
    _servers = parseServers();
  }
  return _servers;
}

export const env = {
  get DATABASE_URL() {
    return required("DATABASE_URL");
  },
  get APP_URL() {
    return requiredInProduction("APP_URL") || "http://localhost:3000";
  },
  get NEXTAUTH_SECRET() {
    return requiredInProduction("NEXTAUTH_SECRET");
  },
  get AUTH_OIDC_ISSUER() {
    return requiredInProduction("AUTH_OIDC_ISSUER");
  },
  get AUTH_OIDC_CLIENT_ID() {
    return requiredInProduction("AUTH_OIDC_CLIENT_ID");
  },
  get AUTH_OIDC_CLIENT_SECRET() {
    return requiredInProduction("AUTH_OIDC_CLIENT_SECRET");
  },
  get NODE_ENV() {
    return process.env.NODE_ENV ?? "development";
  },
};
