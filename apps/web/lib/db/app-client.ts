import { connection } from "next/server";
import postgres from "postgres";

// The profile's client, on market_scout_app. App-owned state lives in the
// private `app` schema, which the read-only role cannot reach, so profile reads
// and Server Action writes go through this pool and nothing else does. Taxonomy
// search stays on the read-only client.
//
// Server-only. No Client Component may import this module; a DB-free test
// enforces it.

const globalForAppSql = globalThis as typeof globalThis & {
  marketScoutAppSql?: ReturnType<typeof postgres>;
};

function createAppSql() {
  const databaseUrl = process.env.DATABASE_URL_APP;

  if (!databaseUrl) {
    throw new Error("DATABASE_URL_APP must be set for profile reads and writes");
  }

  return postgres(databaseUrl, {
    max: 3,
    connection: {
      application_name: "market-scout-web-app",
    },
  });
}

export async function getAppSql() {
  // Same ordering as getSql(): mark the caller dynamic before reading the DSN.
  await connection();

  globalForAppSql.marketScoutAppSql ??= createAppSql();

  return globalForAppSql.marketScoutAppSql;
}
