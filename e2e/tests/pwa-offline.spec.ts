import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';
import { createTestCard, registerTestUser, seedAuthenticatedPage } from './helpers';

/**
 * L9 — PWA offline mode acceptance.
 *
 * From plan §6.4: "50 reviews complete in airplane mode; events sync on
 * reconnect." This test seeds 50 cards, switches the browser context to
 * offline, plays 50 reviews via keyboard shortcuts, asserts the local
 * IndexedDB queue depth = 50, then goes online and asserts the queue
 * drains within five seconds while the server-side review-event count
 * climbs to 50.
 *
 * Runs only against Chromium-family browsers — SafariWebKit doesn't
 * yet expose context.setOffline behavior that the SW respects.
 */
test('offline-mode review queues 50 events and flushes on reconnect', async ({ page, request, context, browserName }) => {
  test.skip(browserName === 'webkit', 'WebKit doesn\'t expose setOffline behaviour for SWs in Playwright');

  const { apiBase, access_token } = await registerTestUser(request, 'off', 'Offline');

  for (let i = 0; i < 50; i++) {
    await createTestCard(request, apiBase, access_token, {
      lemma: `単語${i}`,
      backText: `word ${i}`,
    });
  }

  await seedAuthenticatedPage(page, access_token);
  await page.goto('/review');

  // Wait for the first card to be visible before going offline.
  await page.waitForSelector('.flashcard, .card-front, .word', { timeout: 5_000 });
  await expectNoSeriousA11y(page, { label: 'review (online)' });

  await context.setOffline(true);

  for (let i = 0; i < 50; i++) {
    await page.keyboard.press(' ');
    await page.waitForTimeout(40);
    await page.keyboard.press('3');
    await page.waitForTimeout(60);
  }

  // Read the IndexedDB queue depth that lib/offline.ts populates.
  const queueDepth = async () => page.evaluate(async () => {
    return new Promise<number>((resolve) => {
      const req = indexedDB.open('carve_offline');
      req.onsuccess = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains('review_events')) return resolve(0);
        const tx = db.transaction('review_events', 'readonly');
        const count = tx.objectStore('review_events').count();
        count.onsuccess = () => resolve(count.result);
        count.onerror = () => resolve(-1);
      };
      req.onerror = () => resolve(-1);
    });
  });
  await expect.poll(queueDepth, { timeout: 10_000 }).toBe(50);

  // Reconnect; the page listens for `online` and calls flushQueue.
  await context.setOffline(false);
  await expect.poll(queueDepth, { timeout: 10_000 }).toBe(0);

  // The mock exposes a test-only count so this acceptance test proves both
  // durable queue drain and exactly-once server submission.
  await expect.poll(async () => {
    const response = await request.get('http://localhost:8080/__test/review-events', {
      headers: { Authorization: `Bearer ${access_token}` },
    });
    const body = await response.json() as { count: number; unique_event_ids: number };
    return body;
  }, { timeout: 10_000 }).toEqual({ count: 50, unique_event_ids: 50 });
});
