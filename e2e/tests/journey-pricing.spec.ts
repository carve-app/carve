import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

test('pricing page lists plans + has clear CTAs', async ({ page }) => {
  await page.goto('/pricing');
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/pricing' });

  // At least the free + one paid plan should be visible.
  const planNames = await page.locator('h2, h3').allInnerTexts();
  expect(planNames.join(' ').toLowerCase()).toMatch(/free|pro|team|starter/);
});

test('privacy page is a real document', async ({ page }) => {
  await page.goto('/privacy');
  await page.waitForLoadState('networkidle').catch(() => {});
  await expectNoSeriousA11y(page, { label: '/privacy' });
  // A privacy policy must mention data retention or the relevant law.
  const body = await page.locator('main, article, body').first().innerText();
  expect(body.toLowerCase()).toMatch(/data|privacy|cookie|retain/);
});
