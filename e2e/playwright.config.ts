import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright config — drives the L5 (web journeys), L6 (extension),
 * L7 (visual regression), L8 (accessibility), and L9 (PWA offline)
 * layers of the testing strategy.
 *
 * The mock-server in `e2e/mock-server.ts` stands in for the live API +
 * NLP services so journey tests can run in CI without a full
 * docker-compose stack. The "real-services" project (gated by
 * E2E_USE_REAL=1) runs the same tests against testcontainers Postgres +
 * the actual Go binary, and is used only for the nightly job.
 */

const useRealServices = process.env.E2E_USE_REAL === '1';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.WEB_BASE_URL ?? 'http://localhost:5173',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  webServer: useRealServices
    ? [
        {
          command: 'cd ../apps/web && VITE_API_BASE=http://localhost:8080 npm run dev -- --port 5173',
          port: 5173,
          reuseExistingServer: !process.env.CI,
          timeout: 60_000,
        },
      ]
    : [
        {
          command: 'node mock-server.cjs',
          port: 8080,
          reuseExistingServer: !process.env.CI,
          timeout: 30_000,
        },
        {
          command: 'cd ../apps/web && VITE_API_BASE=http://localhost:8080 npm run dev -- --port 5173',
          port: 5173,
          reuseExistingServer: !process.env.CI,
          timeout: 60_000,
        },
      ],
  projects: [
    {
      name: 'web-chromium',
      use: { ...devices['Desktop Chrome'] },
    },
    {
      name: 'web-firefox',
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'web-webkit',
      use: { ...devices['Desktop Safari'] },
    },
    {
      name: 'web-mobile',
      use: { ...devices['iPhone 14'] },
    },
  ],
});
