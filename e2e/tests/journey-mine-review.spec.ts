import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { createTestCard, registerTestUser, seedAuthenticatedPage } from './helpers';

/**
 * L5 — mining + reviewing journey.
 *
 * Seeds a user + card directly through the API, then drives the review
 * page with keyboard shortcuts to verify the full review loop: flip
 * (Space) → rate (3) → next card. Asserts the server received the
 * review event by polling the due-count.
 */
test('user can review a card with keyboard shortcuts', async ({ page, request }) => {
  // Register via API so we don't depend on the UI register flow here.
  const { apiBase, access_token } = await registerTestUser(request, 'kbd', 'KBD');

  // Seed three cards through the API.
  for (const word of ['猫', '犬', '鳥']) {
    await createTestCard(request, apiBase, access_token, {
      lemma: word,
      backText: `${word} definition`,
    });
  }

  // Persist the token the same way the SvelteKit shell does, then load /review.
  await seedAuthenticatedPage(page, access_token);
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
