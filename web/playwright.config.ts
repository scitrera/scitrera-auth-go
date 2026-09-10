// SPDX-License-Identifier: AGPL-3.0-only
import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  workers: 1,
  fullyParallel: false,
  timeout: 60000,
  reporter: [["list"]],
  outputDir: process.env.AUTH_BROWSER_OUTPUT ?? "test-results",
  use: {
    baseURL: process.env.AUTH_BROWSER_URL ?? "http://127.0.0.1:9082",
    browserName: "chromium",
    ignoreHTTPSErrors: process.env.AUTH_BROWSER_INSECURE_TLS === "1",
    launchOptions: { executablePath: process.env.AUTH_CHROMIUM_PATH },
    // Traces/network logs would capture the operator token during login.
    trace: "off",
    video: "off",
    screenshot: "only-on-failure",
  },
});
