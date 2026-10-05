import { afterEach, describe, expect, it, vi } from "vitest";

// getAppSql() awaits connection(), which never resolves outside a request
// scope. The stub lets the test reach the DSN check that follows it.
vi.mock("next/server", () => ({
  connection: async () => {},
}));

const originalDatabaseUrl = process.env.DATABASE_URL_APP;

afterEach(() => {
  // Env assignment coerces to string, so restoring an absent value would set
  // the literal "undefined" -- a truthy DSN for any later import.
  if (originalDatabaseUrl === undefined) {
    delete process.env.DATABASE_URL_APP;
  } else {
    process.env.DATABASE_URL_APP = originalDatabaseUrl;
  }
  vi.resetModules();
});

describe("getAppSql", () => {
  it("names DATABASE_URL_APP when the app DSN is unset or empty", async () => {
    for (const value of [undefined, ""]) {
      if (value === undefined) {
        delete process.env.DATABASE_URL_APP;
      } else {
        process.env.DATABASE_URL_APP = value;
      }
      vi.resetModules();
      const { getAppSql } = await import("./app-client");

      await expect(getAppSql()).rejects.toThrow("DATABASE_URL_APP must be set");
    }
  });
});
