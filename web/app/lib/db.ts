import "server-only";
import { drizzle } from "drizzle-orm/node-postgres";
import { Pool } from "pg";
import * as schema from "./db/schema";
import { env } from "./env";

// Cache the connection on globalThis to survive Next.js HMR in development.
// Without this, every hot-reload creates a new pool, quickly exhausting
// PostgreSQL's max_connections limit.
const globalForDb = globalThis as unknown as { pool: Pool | undefined };

function createPool(): Pool {
  return new Pool({
    connectionString: env.DATABASE_URL,
    max: 10,
    idleTimeoutMillis: 20_000,
  });
}

const pool = globalForDb.pool ?? createPool();

if (process.env.NODE_ENV !== "production") {
  globalForDb.pool = pool;
}

export const db = drizzle(pool, { schema });
