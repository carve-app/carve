import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { registerTestUser, seedAuthenticatedPage } from './helpers';

test('language switcher persists across navigation', async ({ page, request }) => {
  const { access_token } = await registerTestUser(request, 'lang', 'Lang Tester');
  await seedAuthenticatedPage(page, access_token);

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
