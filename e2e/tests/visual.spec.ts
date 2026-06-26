import { test, expect } from '@playwright/test';

/**
 * L7 — visual regression baselines.
 *
 * One screenshot per public route in the Chromium visual project. Functional
 * and accessibility journeys still run across the complete browser matrix;
 * keeping visual baselines on one engine avoids treating platform font
 * rendering differences as application regressions. Baselines live under
 * e2e/tests/visual.spec.ts-snapshots/. To accept new
 * baselines after intentional changes, run:
 *
 *     pnpm exec playwright test visual.spec.ts --project=web-chromium --update-snapshots
 *
 * and commit the resulting files.
 */
const ROUTES = [
  { name: 'landing',         path: '/' },
  { name: 'login',           path: '/login' },
  { name: 'register',        path: '/register' },
  { name: 'pricing',         path: '/pricing' },
  { name: 'privacy',         path: '/privacy' },
  { name: 'forgot-password', path: '/forgot-password' },
];

test.beforeEach(({ browserName }) => {
  test.skip(browserName !== 'chromium', 'Visual baselines are maintained for Chromium');
});

for (const { name, path } of ROUTES) {
  test(`route ${name} renders consistently`, async ({ page }) => {
    await page.goto(path);
    await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {});
    await page.waitForTimeout(200);
    await expect(page).toHaveScreenshot(`${name}.png`, {
      fullPage: true,
      maxDiffPixelRatio: 0.02,
      animations: 'disabled',
    });
  });
}
