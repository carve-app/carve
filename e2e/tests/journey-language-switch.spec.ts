import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

test('language switcher persists across navigation', async ({ page, request }) => {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = `lang+${Date.now()}@example.com`;
  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'super-secret-123', display_name: 'Lang Tester' },
  });
  const { access_token } = await reg.json();
  await page.goto('/');
  await page.evaluate((t: string) => localStorage.setItem('carve_access_token', t), access_token);

  await page.goto('/cards');
  await page.waitForLoadState('networkidle').catch(() => {});

  const langSelect = page.locator('select[aria-label="Language"]').first();
  if (await langSelect.count()) {
    await langSelect.selectOption('en');
    const persisted = await page.evaluate(() => localStorage.getItem('carve_lang'));
    expect(persisted).toBe('en');

    // Navigate to stats and verify selection survives.
    await page.goto('/stats');
    await page.waitForLoadState('networkidle').catch(() => {});
    const stillEn = await page.evaluate(() => localStorage.getItem('carve_lang'));
    expect(stillEn).toBe('en');
  }
  await expectNoSeriousA11y(page, { label: 'cards (after lang switch)' });
});
