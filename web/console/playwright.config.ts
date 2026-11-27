import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "e2e",
  timeout: 240_000,
  expect: { timeout: 90_000 },
  workers: 1,
  reporter: [["list"], ["html", { open: "never", outputFolder: process.env.PW_REPORT ?? "playwright-report" }], ["junit", { outputFile: process.env.PW_JUNIT ?? "test-results/junit.xml" }]],
  use: {
    baseURL: process.env.BASE_URL ?? "http://127.0.0.1:5173",
    viewport: { width: 1280, height: 860 },
    trace: "off",
  },
});
