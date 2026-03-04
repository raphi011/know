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
  /** Backend server URL. In auth mode, shown as default on the login page.
   *  In no-auth mode (AUTH_DISABLED=true), used as the sole backend URL. */
  get BACKEND_URL() {
    return process.env.BACKEND_URL || "http://localhost:8484";
  },
  /** Disables all authentication. Only for local/Docker deployments. */
  get AUTH_DISABLED() {
    const disabled = process.env.AUTH_DISABLED === "true";
    if (disabled && process.env.NODE_ENV === "production" && !isBuildPhase) {
      console.warn(
        "[env] WARNING: AUTH_DISABLED=true in production — all authentication is bypassed!",
      );
    }
    return disabled;
  },
  get NODE_ENV() {
    return process.env.NODE_ENV ?? "development";
  },
};
