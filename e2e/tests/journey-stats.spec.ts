import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

test('stats page renders with chart/heatmap primitives', async ({ page, request }) => {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = `stats+${Date.now()}@example.com`;
  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'super-secret-123', display_name: 'Stats Tester' },
  });
  const { access_token } = await reg.json();
  await page.goto('/');
  await page.evaluate((t: string) => localStorage.setItem('carve_access_token', t), access_token);

  await page.goto('/stats');
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/stats' });

  // At least one of the chart primitives must render. They emit role=img.
  const svgImgs = page.locator('svg[role="img"]');
  await expect(svgImgs.first()).toBeVisible({ timeout: 5_000 });
});
