import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

test('verify-email page handles missing token gracefully', async ({ page }) => {
  await page.goto('/verify-email');
  await expectNoSeriousA11y(page, { label: 'verify-email (no token)' });
  await expect(page.locator('text=/no verification token|couldn\'t verify|resend/i').first()).toBeVisible();
  // Resend form should let us submit any email and reply ok (anti-enumeration).
  const resendInput = page.locator('input[type="email"]');
  if (await resendInput.count()) {
    await resendInput.fill('someone@example.com');
    await page.click('button[type="submit"]');
    await expect(page.locator('text=/sent|fresh|verification|email/i').first()).toBeVisible({ timeout: 5_000 });
  }
});

test('verify-email shows error with invalid token', async ({ page }) => {
  await page.goto('/verify-email?token=invalid-token-xyz');
  await expectNoSeriousA11y(page, { label: 'verify-email (bad token)' });
  await expect(page.locator('text=/couldn\'t verify|invalid|expired|resend/i').first()).toBeVisible({ timeout: 5_000 });
});
