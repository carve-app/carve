import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

test('forgot → reset password flow', async ({ page, request }) => {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = `reset+${Date.now()}@example.com`;
  await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'old-password-123', display_name: 'Reset Tester' },
  });

  await page.goto('/forgot-password');
  await expectNoSeriousA11y(page, { label: 'forgot-password' });

  await page.fill('input[type="email"]', email);
  await page.click('button[type="submit"]');
  await expect(page.locator('text=/check your email|sent|link/i').first()).toBeVisible({ timeout: 5_000 });

  // Direct token use — the mock server accepts the literal "test-reset-token".
  await page.goto('/reset-password?token=test-reset-token');
  await expectNoSeriousA11y(page, { label: 'reset-password' });
  await page.fill('input[type="password"]', 'brand-new-password-456');
  // Some variants have a confirm field.
  const confirm = page.locator('input[name="confirm"], input[name="password_confirm"]').first();
  if (await confirm.count()) await confirm.fill('brand-new-password-456');
  await page.click('button[type="submit"]');
  await expect(page.locator('text=/password|reset|signed in|continue/i').first()).toBeVisible({ timeout: 5_000 });
});
