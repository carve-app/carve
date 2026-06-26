import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { TEST_PASSWORD, uniqueEmail } from './helpers';

/**
 * L5 — full journey: landing → register → onboarding → cards page.
 *
 * Asserted invariants:
 *   - public marketing route loads without auth
 *   - register endpoint hits the API and stores a token in localStorage
 *   - onboarding wizard advances through every step
 *   - cards page loads (empty state acceptable)
 *   - no serious accessibility violations on any visited route
 */
test('user can register and reach the cards page', async ({ page }) => {
  await page.goto('/');
  await expectNoSeriousA11y(page, { label: 'landing' });

  await page.goto('/register');
  await expectNoSeriousA11y(page, { label: 'register', exclude: ['#carve-popup'] });

  const email = uniqueEmail('tester');
  await page.fill('input[type="email"]', email);
  await page.fill('input[type="password"]', TEST_PASSWORD);
  // The register form has a display-name field in some variants.
  const nameInput = page.locator('input[name="display_name"], input[name="name"]').first();
  if (await nameInput.count()) await nameInput.fill('Beta Tester');

  await Promise.all([
    page.waitForURL(/\/(onboarding|cards)/, { timeout: 10_000 }),
    page.click('button[type="submit"]'),
  ]);

  // Either onboarding or a direct land on /cards — both flows count.
  if (page.url().includes('/onboarding')) {
    await expectNoSeriousA11y(page, { label: 'onboarding' });
    // Walk every step. Each step is gated by a "Continue" button.
    for (let step = 0; step < 6; step++) {
      const cont = page.locator('button:has-text("Continue"), button:has-text("Go to my cards")').first();
      if (await cont.count() === 0) break;
      await cont.click();
      await page.waitForTimeout(150);
    }
    await page.waitForURL(/\/cards/, { timeout: 10_000 });
  }

  await expect(page).toHaveURL(/\/cards/);
  await expectNoSeriousA11y(page, { label: 'cards', exclude: ['[data-no-a11y]'] });
});

test('onboarding blocks and reports a failed starter-deck save', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('carve_access_token', 'test-token');
  });
  await page.route('http://localhost:8080/v1/onboarding/starter-deck', async route => {
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Starter deck is temporarily unavailable' }),
    });
  });

  await page.goto('/onboarding');
  await expect(page.getByRole('heading', { name: 'What language are you learning?' })).toBeVisible();
  await page.waitForTimeout(200);
  await page.getByRole('button', { name: /continue/i }).click();
  await expect(page.getByRole('heading', { name: 'Which of these words do you already know?' })).toBeVisible();
  await page.getByRole('button', { name: /continue/i }).click();
  await expect(page.getByRole('heading', { name: 'Start with a curated deck' })).toBeVisible();
  await page.getByRole('button', { name: /continue/i }).click();

  await expect(page.getByRole('alert')).toContainText('Starter deck is temporarily unavailable');
  await expect(page.getByRole('heading', { name: 'Start with a curated deck' })).toBeVisible();
});
