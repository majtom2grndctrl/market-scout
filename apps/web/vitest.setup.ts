import { loadEnvConfig } from "@next/env";

const environment = process.env as Record<string, string | undefined>;
const nodeEnv = environment.NODE_ENV;
// Avoid Next's test-mode file set, which excludes .env.local; restore process state after loading.
environment.NODE_ENV = "development";
loadEnvConfig(process.cwd());
if (nodeEnv === undefined) {
  delete environment.NODE_ENV;
} else {
  environment.NODE_ENV = nodeEnv;
}
