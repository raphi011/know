import { drizzle } from "drizzle-orm/node-postgres";
import { Pool } from "pg";
import * as schema from "../app/lib/db/schema";

const DATABASE_URL =
  process.env.DATABASE_URL ?? "postgresql://app:app@localhost:5433/app_dev";

async function seed() {
  const pool = new Pool({ connectionString: DATABASE_URL });
  const db = drizzle(pool, { schema });

  console.log("Seeding database...");

  // Create example users
  const rows = await db
    .insert(schema.users)
    .values([
      { id: "user-1", name: "Alice Johnson", email: "alice@example.com" },
      { id: "user-2", name: "Bob Smith", email: "bob@example.com" },
    ])
    .returning();

  const alice = rows[0]!;
  const bob = rows[1]!;

  console.log(`Created users: ${alice.name}, ${bob.name}`);

  await pool.end();
  console.log("Seed complete!");
}

seed().catch((err) => {
  console.error("Seed failed:", err);
  process.exit(1);
});
