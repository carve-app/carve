import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { registerTestUser, seedAuthenticatedPage } from './helpers';

test('stats page renders with chart/heatmap primitives', async ({ page, request }) => {
  const { access_token } = await registerTestUser(request, 'stats', 'Stats Tester');
  await seedAuthenticatedPage(page, access_token);

  await page.goto('/stats');
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/stats' });

  // At least one of the chart primitives must render. They emit role=img.
  const svgImgs = page.locator('svg[role="img"]');
  await expect(svgImgs.first()).toBeVisible({ timeout: 5_000 });
});
