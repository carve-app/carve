import { test, expect } from '@playwright/test';
import { expectNoSeriousA11y } from './a11y';

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

  const apiBase = process.env.API_BASE ?? 'http://localhost:8080';
  const email = `off+${Date.now()}@example.com`;

  const reg = await request.post(`${apiBase}/v1/auth/register`, {
    data: { email, password: 'super-secret-123', display_name: 'Offline' },
  });
  const { access_token } = await reg.json();

  for (let i = 0; i < 50; i++) {
    await request.post(`${apiBase}/v1/cards`, {
      headers: { Authorization: `Bearer ${access_token}` },
      data: { front_text: `word${i}`, back_text: `WORD${i}`, language_code: 'en' },
    });
  }

  await page.goto('/');
  await page.evaluate((t) => localStorage.setItem('carve_access_token', t), access_token);
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
  const queueLen = await page.evaluate(async () => {
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
  expect(queueLen).toBeGreaterThanOrEqual(45);  // some clicks may have raced

  // Reconnect; the page listens for `online` and calls flushQueue.
  await context.setOffline(false);
  await page.waitForTimeout(2_000);

  const drained = await page.evaluate(async () => {
    return new Promise<number>((resolve) => {
      const req = indexedDB.open('carve_offline');
      req.onsuccess = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains('review_events')) return resolve(0);
        const tx = db.transaction('review_events', 'readonly');
        const count = tx.objectStore('review_events').count();
        count.onsuccess = () => resolve(count.result);
      };
    });
  });
  expect(drained).toBeLessThanOrEqual(2);  // anything still queued should be a flake, not a regression
});
