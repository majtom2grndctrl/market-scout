import { connection } from "next/server";
import postgres from "postgres";

const globalForSql = globalThis as typeof globalThis & {
  marketScoutSql?: ReturnType<typeof postgres>;
};

function createSql() {
  const databaseUrl = process.env.DATABASE_URL_RO;

  if (!databaseUrl) {
    throw new Error("DATABASE_URL_RO must be set for web database access");
  }

  return postgres(databaseUrl, {
    max: 5,
    connection: {
      application_name: "market-scout-web",
    },
  });
}

export async function getSql() {
  // connection() comes first, before the DSN is read. It marks the caller
  // dynamic, so a build-time prerender bails out here and never reaches the
  // check — the missing-DSN error then only fires inside a real request,
  // where the DSN is genuinely absent rather than merely not yet injected.
  await connection();

  // Reuse the pool across Next development reloads to avoid extra connections.
  globalForSql.marketScoutSql ??= createSql();

  return globalForSql.marketScoutSql;
}
