import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { registerTestUser, seedAuthenticatedPage } from './helpers';

const ROUTES = ['/cards', '/decks', '/library', '/stats', '/output', '/settings'];

test('app shell — every primary route loads under one token', async ({ page, request }) => {
  const { access_token } = await registerTestUser(request, 'shell', 'Shell Tester');
  await seedAuthenticatedPage(page, access_token);

  for (const r of ROUTES) {
    await page.goto(r);
    await page.waitForLoadState('networkidle').catch(() => {});
    await expectNoSeriousA11y(page, { label: `shell-nav: ${r}` });
    // Layout chrome should be present on every route.
    await expect(page.locator('select[aria-label="Language"]').first()).toBeVisible({ timeout: 3_000 });
  }
});
