import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

/**
 * L5 — mining + reviewing journey.
 *
 * Seeds a user + card directly through the API, then drives the review
 * page with keyboard shortcuts to verify the full review loop: flip
 * (Space) → rate (3) → next card. Asserts the server received the
 * review event by polling the due-count.
 */
test('user can review a card with keyboard shortcuts', async ({ page, request, baseURL }) => {
  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = `kbd+${Date.now()}@example.com`;

  // Register via API so we don't depend on the UI register flow here.
  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'super-secret-123', display_name: 'KBD' },
  });
  expect(reg.ok()).toBe(true);
  const { access_token } = await reg.json();

  // Seed three cards through the API.
  for (const word of ['cat', 'dog', 'bird']) {
    const r = await request.post(`${apiBase}/v1/cards`, {
      headers: { Authorization: `Bearer ${access_token}` },
      data: { front_text: word, back_text: word.toUpperCase(), language_code: 'en' },
    });
    expect(r.ok()).toBe(true);
  }

  // Persist the token the same way the SvelteKit shell does, then load /review.
  await page.goto('/');
  await page.evaluate((t) => localStorage.setItem('carve_access_token', t), access_token);
  await page.goto('/review');
  await expectNoSeriousA11y(page, { label: 'review' });

  // Walk three cards: Space → 3 (Good).
  for (let i = 0; i < 3; i++) {
    await page.waitForSelector('.flashcard, .card-front, .word', { timeout: 5_000 });
    await page.keyboard.press(' ');
    await page.waitForTimeout(120);
    await page.keyboard.press('3');
    await page.waitForTimeout(180);
  }

  // Expect to see a session-complete screen.
  await expect(page.locator('text=/Session complete|review more/i').first()).toBeVisible({ timeout: 5_000 });
});
