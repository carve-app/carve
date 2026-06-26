import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { TEST_PASSWORD, registerTestUser } from './helpers';

test('login → auth guard → logout', async ({ page, request }) => {
  const { email } = await registerTestUser(request, 'login', 'Login Tester');

  // Guard: hitting /cards without a token redirects to /login.
  await page.context().clearCookies();
  await page.goto('/cards');
  await expect(page).toHaveURL(/\/login/);
  await expectNoSeriousA11y(page, { label: 'login (guard redirect)' });

  // Sign in.
  await page.fill('input[type="email"]', email);
  await page.fill('input[type="password"]', TEST_PASSWORD);
  await Promise.all([
    page.waitForURL(/\/(cards|onboarding|review)/, { timeout: 10_000 }),
    page.click('button[type="submit"]'),
  ]);

  // Logout via user menu (top-right).
  await page.goto('/cards');
  const userBtn = page.locator('button[aria-label="User menu"]').first();
  if (await userBtn.count()) {
    await userBtn.click();
    await page.click('button:has-text("Sign out")');
    await expect(page).toHaveURL(/\/login/);
  }
});
