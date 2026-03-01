import "server-only";

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
  get KNOWHOW_API_URL() {
    return required("KNOWHOW_API_URL");
  },
  get KNOWHOW_API_TOKEN() {
    return required("KNOWHOW_API_TOKEN");
  },
  get NODE_ENV() {
    return process.env.NODE_ENV ?? "development";
  },
};
