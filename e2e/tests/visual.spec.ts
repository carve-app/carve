import { test, expect } from '@playwright/test';

/**
 * L7 — visual regression baselines.
 *
 * One screenshot per public route per browser. Baselines live under
 * e2e/tests/__screenshots__/visual.spec.ts-snapshots/. To accept new
 * baselines after intentional changes, run:
 *
 *     npx playwright test visual.spec.ts --update-snapshots
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
