import { Pool } from "pg";
import { afterAll, beforeAll } from "vitest";

let pool: Pool;

beforeAll(async () => {
  const connectionString =
    process.env.DATABASE_URL ?? "postgresql://app:app@localhost:5433/app_dev";

  pool = new Pool({ connectionString, max: 1 });

  // Verify connectivity
  const client = await pool.connect();
  client.release();
});

afterAll(async () => {
  await pool?.end();
});
