import { resolve } from "node:path";

import { defineConfig, configDefaults } from "vitest/config";

export default defineConfig(({ mode }) => {
  const databaseTests = mode === "db";

  return {
    resolve: {
      alias: { "@": resolve(__dirname, ".") },
    },
    test: {
      environment: "node",
      exclude: databaseTests
        ? configDefaults.exclude
        : [...configDefaults.exclude, "**/*.db.test.{ts,tsx}"],
      include: databaseTests ? ["**/*.db.test.{ts,tsx}"] : ["**/*.test.{ts,tsx}"],
      setupFiles: ["./vitest.setup.ts"],
    },
  };
});
