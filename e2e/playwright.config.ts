import { defineConfig, devices } from "@playwright/test";

const chromiumURL = "http://127.0.0.1:17391";
const firefoxURL = "http://127.0.0.1:17392";
const webkitURL = "http://127.0.0.1:17393";

export default defineConfig({
  testDir: "./tests",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 3,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  globalSetup: "./global-setup.ts",
  globalTeardown: "./global-teardown.ts",
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    trace: "on-first-retry",
    viewport: { width: 1280, height: 800 },
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"], baseURL: chromiumURL } },
    { name: "firefox", use: { ...devices["Desktop Firefox"], baseURL: firefoxURL } },
    { name: "webkit", use: { ...devices["Desktop Safari"], baseURL: webkitURL } },
  ],
});
