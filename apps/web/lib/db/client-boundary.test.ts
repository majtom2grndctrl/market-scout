import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

import { describe, expect, it } from "vitest";

// No Client Component imports a database client. A client module that pulled
// in lib/db would ship the module graph toward the browser bundle -- for the
// app client, the code path that reaches the private profile schema. Type-only
// imports are erased at build and are allowed; Server Actions ("use server")
// are server code and are not client modules.
//
// Direct imports only: a client module importing a helper that itself imports
// lib/db would pass. lib/db is imported only by Server Components and actions
// today, so the direct check is the one that can fail.

const ROOT = join(__dirname, "..", "..");
const SCANNED = ["app", "components", "hooks", "lib"];

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "node_modules" ? [] : sourceFiles(path);
    }
    return /\.(ts|tsx)$/.test(entry.name) ? [path] : [];
  });
}

function isClientModule(source: string): boolean {
  const firstStatement = source.replace(/^(\s|\/\/[^\n]*\n|\/\*[\s\S]*?\*\/)*/, "");
  return /^["']use client["']/.test(firstStatement);
}

const VALUE_IMPORT_FROM_DB =
  /^\s*import\s+(?!type\s)[^;]*?from\s+["']((?:@\/lib\/db|(?:\.{1,2}\/)+(?:lib\/)?db)(?:\/[^"']*)?|\.\/(?:app-)?client|\.\/profile|\.\/taxonomy-search)["']/m;

describe("client boundary", () => {
  it("has no client module importing a value from lib/db", () => {
    const offenders = SCANNED.flatMap((dir) => sourceFiles(join(ROOT, dir)))
      .filter((file) => {
        const source = readFileSync(file, "utf8");
        return isClientModule(source) && VALUE_IMPORT_FROM_DB.test(source);
      })
      .map((file) => relative(ROOT, file));

    expect(offenders).toEqual([]);
  });

  it("recognizes a client module importing the app client", () => {
    const source = `"use client";\n\nimport { getAppSql } from "@/lib/db/app-client";\n`;
    expect(isClientModule(source) && VALUE_IMPORT_FROM_DB.test(source)).toBe(true);

    const typeOnly = `"use client";\nimport type { Profile } from "@/lib/db/profile";\n`;
    expect(VALUE_IMPORT_FROM_DB.test(typeOnly)).toBe(false);
  });
});
