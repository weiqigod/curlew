import { defineConfig, devices } from '@playwright/test';

// Playwright e2e against the REAL `apitest ui` binary (UI_SPECIFICATION.md §13.3).
//
// There is deliberately NO `webServer` block: the server port and the session
// token are dynamic (`--port 0` prints them to stdout), so the global setup
// owns the whole lifecycle — SPA build freshness, Go binary build, the local
// echo API, the fixture project, and one `apitest ui` process.
// See tests/e2e/global-setup.ts.
//
// Workers are pinned to 1: the server enforces single-flight runs
// (409 run_active), so tests within and across specs must not overlap.
export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : [['list']],
  globalSetup: './tests/e2e/global-setup.ts',
  globalTeardown: './tests/e2e/global-teardown.ts',
  use: {
    trace: 'on-first-retry',
    viewport: { width: 1280, height: 800 },
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
