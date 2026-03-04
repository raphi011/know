import "server-only";

// During `next build`, env vars aren't available.
// Skip validation so static page generation can complete.
const isBuildPhase = process.env.NEXT_PHASE === "phase-production-build";

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
  get APP_URL() {
    return requiredInProduction("APP_URL") || "http://localhost:3000";
  },
  /** Secret used to encrypt session cookies. Required in production. */
  get SESSION_SECRET() {
    return requiredInProduction("SESSION_SECRET");
  },
  /** Optional default server URL shown on the login page. */
  get BACKEND_URL() {
    return process.env.BACKEND_URL || "http://localhost:8484";
  },
  get AUTH_DISABLED() {
    return process.env.AUTH_DISABLED === "true";
  },
  get NODE_ENV() {
    return process.env.NODE_ENV ?? "development";
  },
};
