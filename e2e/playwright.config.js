import { defineConfig } from "@playwright/test";

// HYDRASCALE_E2E_BASE_URL points the suite at any environment. It defaults to the
// tunnel address the review sandbox used, so a developer with the same tunnel open
// needs to set nothing.
const baseURL = process.env.HYDRASCALE_E2E_BASE_URL ?? "http://127.0.0.1:9443";

export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  retries: 0,
  reporter: [["list"]],
  use: {
    baseURL,
    trace: "retain-on-failure",
  },
});
