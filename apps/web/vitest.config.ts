import { defineConfig, configDefaults } from "vitest/config";

export default defineConfig(({ mode }) => {
  const databaseTests = mode === "db";

  return {
    test: {
      environment: "node",
      exclude: databaseTests
        ? configDefaults.exclude
        : [...configDefaults.exclude, "**/*.db.test.ts"],
      include: databaseTests ? ["**/*.db.test.ts"] : ["**/*.test.ts"],
      setupFiles: ["./vitest.setup.ts"],
    },
  };
});
