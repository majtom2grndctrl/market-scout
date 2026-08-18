import { afterEach, describe, expect, it, vi } from "vitest";

// getSql() awaits connection(), which never resolves outside a request scope.
// The stub lets the test reach the DSN check that follows it.
vi.mock("next/server", () => ({
  connection: async () => {},
}));

const originalDatabaseUrl = process.env.DATABASE_URL_RO;

afterEach(() => {
  // Env assignment coerces to string, so restoring an absent value would set
  // the literal "undefined" — a truthy DSN for any later import.
  if (originalDatabaseUrl === undefined) {
    delete process.env.DATABASE_URL_RO;
  } else {
    process.env.DATABASE_URL_RO = originalDatabaseUrl;
  }
  vi.resetModules();
});

describe("getSql", () => {
  it("names DATABASE_URL_RO when the read-only DSN is empty", async () => {
    process.env.DATABASE_URL_RO = "";
    vi.resetModules();

    const { getSql } = await import("./client");

    await expect(getSql()).rejects.toThrow("DATABASE_URL_RO");
  });

  it("throws when the read-only DSN is unset", async () => {
    delete process.env.DATABASE_URL_RO;
    vi.resetModules();

    const { getSql } = await import("./client");

    await expect(getSql()).rejects.toThrow(
      "DATABASE_URL_RO must be set for web database access",
    );
  });
});
