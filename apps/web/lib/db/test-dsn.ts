// A test database is identified by its name, not by which variable carried it:
// the guard below has to reject the development DSN even when it arrives under
// the right variable name.
const TEST_DATABASE_SUFFIX = "_test";

/**
 * DSNs for the `.db.test.ts` suites, resolved from the dedicated test database.
 *
 * There is deliberately no fallback to DATABASE_URL / DATABASE_URL_RO: fixture
 * rows written to the development database have leaked past best-effort
 * teardown, and every fixture company became a permanent fetcher target. A
 * suite with no test DSN skips rather than reaching the shared database.
 *
 * Unset and misconfigured are different conditions. Unset stays a skip, so a
 * developer who has not provisioned the test database is not blocked — the
 * skip decision needs the per-test `context`, so it stays at the call site. A
 * DSN that is set but names a non-test database throws: the test and
 * development variables differ by one word, and a DSN pasted from the wrong one
 * reproduces exactly the leak above while every suite still passes, because
 * assertions here are marker-scoped or delta-based and cannot see it.
 */
export function testDsns(): { ownerDsn?: string; readOnlyDsn?: string } {
  return {
    ownerDsn: testDsn("DATABASE_URL_TEST", process.env.DATABASE_URL_TEST),
    readOnlyDsn: testDsn("DATABASE_URL_TEST_RO", process.env.DATABASE_URL_TEST_RO),
  };
}

function testDsn(variable: string, dsn: string | undefined): string | undefined {
  // An empty value is how a shell suppresses a .env.local key, so it means
  // unset, not misconfigured.
  if (dsn === undefined || dsn.trim() === "") {
    return undefined;
  }

  const database = databaseName(dsn);

  // Unverifiable is treated as wrong: the point of the guard is that no fixture
  // write happens against a database this function could not name.
  if (database === undefined) {
    throw new Error(
      `${variable} is not a Postgres URL naming a database, so it cannot be verified as a test DSN. ` +
        `Expected a URL whose path names a database ending in "${TEST_DATABASE_SUFFIX}", such as market_scout_test. ` +
        `Leave ${variable} unset to skip the database tests.`,
    );
  }

  if (!database.endsWith(TEST_DATABASE_SUFFIX)) {
    throw new Error(
      `${variable} points at database "${database}", which is not a test database. ` +
        `Database tests write fixtures that survive teardown failure, so the DSN must name a database ending in "${TEST_DATABASE_SUFFIX}" — market_scout_test is the provisioned one. ` +
        `Leave ${variable} unset to skip the database tests.`,
    );
  }

  return dsn;
}

function databaseName(dsn: string): string | undefined {
  let url: URL;
  try {
    url = new URL(dsn);
  } catch {
    return undefined;
  }

  const path = url.pathname.replace(/^\//, "");
  if (path === "") {
    return undefined;
  }

  try {
    return decodeURIComponent(path);
  } catch {
    return path;
  }
}
