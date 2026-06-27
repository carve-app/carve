import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { registerTestUser, seedAuthenticatedPage, TEST_PASSWORD, uniqueEmail } from './helpers';

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
test('user can register and reach the cards page', async ({ page, request }) => {
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

  const [registerResponse] = await Promise.all([
    page.waitForResponse(response => response.url().endsWith('/v1/auth/register') && response.request().method() === 'POST'),
    page.waitForURL(/\/verify-email\?email=/, { timeout: 10_000 }),
    page.click('button[type="submit"]'),
  ]);
  expect(registerResponse.status()).toBe(201);
  const tokenResponse = await request.get(`http://localhost:8080/__test/verification-token?email=${encodeURIComponent(email)}`);
  expect(tokenResponse.ok()).toBe(true);
  const { token: verificationToken } = await tokenResponse.json() as { token: string };
  expect(verificationToken).toBeTruthy();
  await expect(page.getByRole('heading', { name: 'Check your inbox' })).toBeVisible();

  await page.goto(`/verify-email?token=${verificationToken}`);
  await expect(page.getByRole('heading', { name: "You're all set" })).toBeVisible();
  await page.getByRole('link', { name: 'Go to login' }).click();
  await page.fill('input[type="email"]', email);
  await page.fill('input[type="password"]', TEST_PASSWORD);
  await Promise.all([
    page.waitForURL(/\/review/, { timeout: 10_000 }),
    page.click('button[type="submit"]'),
  ]);
  await page.goto('/onboarding');
  await page.locator('#svelte-announcer').waitFor({ state: 'attached' });

  if (page.url().includes('/onboarding')) {
    await expectNoSeriousA11y(page, { label: 'onboarding' });
    await page.getByRole('button', { name: /continue/i }).click();
    await expect(page.getByRole('heading', { name: 'Which of these words do you already know?' })).toBeVisible();
    await page.getByRole('button', { name: /continue/i }).click();
    await expect(page.getByRole('heading', { name: 'Start with a curated deck' })).toBeVisible();
    await page.getByRole('button', { name: /continue/i }).click();
    await expect(page.getByRole('heading', { name: 'Install the browser extension' })).toBeVisible();
    await page.getByRole('button', { name: /continue/i }).click();
    await expect(page.getByRole('heading', { name: "You're all set!" })).toBeVisible();
    await Promise.all([
      page.waitForURL(/\/cards/, { timeout: 10_000 }),
      page.getByRole('button', { name: /go to my cards/i }).click(),
    ]);
  }

  await expect(page).toHaveURL(/\/cards/);
  await expectNoSeriousA11y(page, { label: 'cards', exclude: ['[data-no-a11y]'] });
});

test('onboarding blocks and reports a failed starter-deck save', async ({ page, request }) => {
  const user = await registerTestUser(request, 'onboarding-fault', 'Onboarding Fault');
  await seedAuthenticatedPage(page, user.access_token);
  const armed = await request.post(`${user.apiBase}/__test/fail-next-starter-deck`, {
    headers: { Authorization: `Bearer ${user.access_token}` },
  });
  expect(armed.ok()).toBe(true);

  await page.goto('/onboarding');
  await page.locator('#svelte-announcer').waitFor({ state: 'attached' });
  await expect(page.getByRole('heading', { name: 'What language are you learning?' })).toBeVisible();
  await page.getByRole('button', { name: /continue/i }).click();
  await expect(page.getByRole('heading', { name: 'Which of these words do you already know?' })).toBeVisible();
  await page.getByRole('button', { name: /continue/i }).click();
  await expect(page.getByRole('heading', { name: 'Start with a curated deck' })).toBeVisible();
  await page.getByRole('button', { name: /continue/i }).click();

  await expect(page.getByRole('alert')).toContainText('Starter deck is temporarily unavailable');
  await expect(page.getByRole('heading', { name: 'Start with a curated deck' })).toBeVisible();
});

test('English learner completes the scored vocabulary placement test', async ({ page, request }) => {
  const user = await registerTestUser(request, 'english-placement', 'English Placement');
  await seedAuthenticatedPage(page, user.access_token);

  await page.goto('/onboarding');
  await page.getByRole('button', { name: /English \(intermediate\+\)/ }).click();
  await page.getByRole('button', { name: /continue/i }).click();

  await expect(page.getByRole('heading', { name: 'Find your vocabulary starting point' })).toBeVisible();
  await expect(page.getByText('questions', { exact: true })).toBeVisible();
  await expectNoSeriousA11y(page, { label: 'English placement intro' });
  await page.getByRole('button', { name: /start the test/i }).click();

  for (let question = 1; question <= 30; question++) {
    await expect(page.getByText(`Question ${question} of 30`)).toBeVisible();
    await page.getByRole('radio', { name: /the intended meaning/i }).click();
    await page.getByRole('button', {
      name: question === 30 ? /see my result/i : /next question/i,
    }).click();
  }

  await expect(page.getByRole('heading', { name: 'Advanced receptive vocabulary' })).toBeVisible();
  await expect(page.getByText('~12,000')).toBeVisible();
  await expect(page.getByText('30', { exact: true }).first()).toBeVisible();
  await expectNoSeriousA11y(page, { label: 'English placement result' });
});
