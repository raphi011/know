import { drizzle } from "drizzle-orm/node-postgres";
import { Pool } from "pg";
import * as schema from "../../app/lib/db/schema";

const DATABASE_URL =
  process.env.DATABASE_URL ?? "postgresql://app:app@localhost:5433/app_dev";

const pool = new Pool({ connectionString: DATABASE_URL });
export const db = drizzle(pool, { schema });

export async function closeDb() {
  await pool.end();
}
