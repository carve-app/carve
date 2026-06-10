import { test, expect, chromium, type BrowserContext } from '@playwright/test';
import path from 'node:path';
import fs from 'node:fs';

/**
 * Browser-driven verification of the video-mining media path.
 *
 * Exercises the real code added for the free-alpha milestone:
 *   - content SubtitleOverlay.mineCurrentCue() → MINE_CARD (create card)
 *   - content attachVideoMedia() → background ATTACH_VIDEO_MEDIA
 *   - background captureVisibleTab + crop (OffscreenCanvas/createImageBitmap)
 *   - background upload to POST /v1/cards/{id}/media against the REAL stack
 *
 * Requires the docker stack (api :8080, media :8002) to be up. We register a
 * real user via the API, drive the extension UI on an intercepted YouTube URL,
 * press "m" to mine, then assert the card carries a fetchable screenshot.
 *
 * NLP is NOT required: with the NLP proxy down, tokenize returns empty and the
 * overlay falls back to mining the raw cue text — the media path is unaffected.
 */

const API = process.env.E2E_API ?? 'http://localhost:8080';
const MEDIA = process.env.E2E_MEDIA ?? 'http://localhost:8002';
const EXTENSION_DIR = path.resolve(__dirname, '..', '..', 'apps', 'extension', 'dist', 'chrome');

// A minimal page served in place of a real YouTube watch page. lang="ja" so the
// content script's language detector enables annotation; the caption container
// and a sized <video> let the YouTube hook fire and the crop have a target.
const YT_FIXTURE = `<!DOCTYPE html>
<html lang="ja">
<head><meta charset="utf-8"><title>YT</title></head>
<body style="margin:0;background:#111">
  <div style="position:absolute;left:40px;top:50px;width:480px;height:270px;
              background:linear-gradient(135deg,#e53935,#1e88e5)">
    <video class="html5-main-video" style="width:480px;height:270px"></video>
  </div>
  <div class="ytp-caption-window-container">
    <span class="ytp-caption-segment"></span>
  </div>
  <p>日本語のテキスト。これはテストページです。映画を見ながら勉強する。</p>
</body>
</html>`;

test.describe('extension — video mining media path', () => {
  test.skip(!fs.existsSync(EXTENSION_DIR), 'extension dist not built — run the chrome build first');

  let ctx: BrowserContext | undefined;
  test.afterEach(async () => { await ctx?.close(); ctx = undefined; });

  test('mining a subtitle cue creates a card with a captured screenshot', async () => {
    // 1. Register a real user and grab a token from the live API.
    const email = `vidmine+${Date.now()}@example.com`;
    const reg = await fetch(`${API}/v1/auth/register`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password: 'alphapassword123', display_name: 'Vid Mine' }),
    });
    expect(reg.ok, `register failed: ${reg.status}`).toBeTruthy();
    const token = (await reg.json()).access_token as string;
    expect(token).toBeTruthy();

    // 2. Launch a real browser with the built extension loaded.
    ctx = await chromium.launchPersistentContext('', {
      headless: false,
      args: [
        `--disable-extensions-except=${EXTENSION_DIR}`,
        `--load-extension=${EXTENSION_DIR}`,
      ],
    });

    // Grab the background service worker and seed auth + API base into storage.
    let [sw] = ctx.serviceWorkers();
    if (!sw) sw = await ctx.waitForEvent('serviceworker');
    await sw.evaluate(async ([t, base]) => {
      await chrome.storage.local.set({ accessToken: t, apiBaseUrl: base, targetLanguage: 'ja' });
    }, [token, API]);

    // 3. Serve the fixture for any youtube.com/watch navigation.
    const page = await ctx.newPage();
    await page.route('**/*', (route) => {
      if (route.request().url().includes('youtube.com/watch')) {
        return route.fulfill({ status: 200, contentType: 'text/html', body: YT_FIXTURE });
      }
      return route.continue();
    });
    await page.goto('https://www.youtube.com/watch?v=carvetest');

    // 4. The content script should mount the subtitle overlay on this URL.
    await page.waitForSelector('#carve-sub-overlay', { timeout: 15_000 });

    // 5. Fire a subtitle cue: the YouTube hook's MutationObserver picks up the
    //    caption text change and calls overlay.onCue(...).
    await page.evaluate(() => {
      const seg = document.querySelector('.ytp-caption-segment');
      if (seg) seg.textContent = '映画を見ながら勉強する';
    });
    // Overlay renders the cue (token spans if NLP is up, raw text otherwise).
    await page.waitForFunction(() => {
      const t = document.getElementById('cso-target');
      return !!t && !t.textContent?.includes('Subtitles will appear here') && (t.textContent ?? '').length > 0;
    }, { timeout: 10_000 });

    // 6. Press "m" to mine the current cue (the real user gesture).
    await page.bringToFront();
    await page.keyboard.press('m');

    // 7. The overlay reports the outcome in #cso-mine-status. Wait for a
    //    terminal "Mined" message (capture is async).
    await page.waitForFunction(() => {
      const el = document.getElementById('cso-mine-status');
      return !!el && /Mined/i.test(el.textContent ?? '');
    }, { timeout: 20_000 });
    const status = await page.locator('#cso-mine-status').textContent();
    expect(status, `mine status was: ${status}`).toMatch(/Mined/i);

    // 8. Assert against the REAL backend: the user now has a card, and it
    //    carries a screenshot URL that is actually fetchable.
    await expect.poll(async () => {
      const list = await fetch(`${API}/v1/cards?language=ja`, {
        headers: { Authorization: `Bearer ${token}` },
      }).then((r) => r.json());
      return (list.cards ?? []).length;
    }, { timeout: 10_000, message: 'card never appeared in backend' }).toBeGreaterThan(0);

    const list = await fetch(`${API}/v1/cards?language=ja`, {
      headers: { Authorization: `Bearer ${token}` },
    }).then((r) => r.json());
    const cardId = list.cards[0].id as string;

    // The media upload is fire-and-forget from the overlay, so poll the card.
    let imageUrl: string | null = null;
    await expect.poll(async () => {
      const card = await fetch(`${API}/v1/cards/${cardId}`, {
        headers: { Authorization: `Bearer ${token}` },
      }).then((r) => r.json());
      imageUrl = card.image_url ?? null;
      return imageUrl;
    }, { timeout: 15_000, message: 'card never got a screenshot URL' }).toBeTruthy();

    // Fetch the stored screenshot (rewrite internal docker host → localhost).
    const fetchable = (imageUrl as unknown as string).replace('media:8002', new URL(MEDIA).host);
    const img = await fetch(fetchable);
    expect(img.status, `screenshot not retrievable at ${fetchable}`).toBe(200);
    const bytes = (await img.arrayBuffer()).byteLength;
    expect(bytes, 'screenshot is empty').toBeGreaterThan(1000);

    await page.screenshot({ path: path.resolve(__dirname, '..', 'extension-video-mine.png') });
  });
});
