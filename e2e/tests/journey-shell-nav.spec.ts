import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

const ROUTES = ['/cards', '/decks', '/library', '/stats', '/output', '/settings'];

test('app shell — every primary route loads under one token', async ({ page, request }) => {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = `shell+${Date.now()}@example.com`;
  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'super-secret-123', display_name: 'Shell Tester' },
  });
  const { access_token } = await reg.json();
  await page.goto('/');
  await page.evaluate((t: string) => localStorage.setItem('carve_access_token', t), access_token);

  for (const r of ROUTES) {
    await page.goto(r);
    await page.waitForLoadState('networkidle').catch(() => {});
    await expectNoSeriousA11y(page, { label: `shell-nav: ${r}` });
    // Layout chrome should be present on every route.
    await expect(page.locator('select[aria-label="Language"]').first()).toBeVisible({ timeout: 3_000 });
  }
});
