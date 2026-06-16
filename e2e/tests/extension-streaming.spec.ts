import { test, expect, chromium } from '@playwright/test';
import path from 'node:path';
import fs from 'node:fs';

/**
 * L6 — extension mining across the recorded streaming-platform fixtures.
 *
 * We load the built extension into a real browser context, navigate to
 * each platform fixture HTML in e2e/fixtures/streaming/, and assert that
 * the SubtitleOverlay receives a cue. This catches:
 *   (a) the build dropping a platform glue file,
 *   (b) a regression in SubtitleHook's URL-routing table,
 *   (c) the overlay failing to render once the hook fires.
 *
 * Chromium and Firefox are tested in separate test functions because the
 * extension-loading API differs between them. Safari is handled by the
 * AppKit wrapper project in apps/extension/safari/ and is run separately
 * via xcodebuild in the nightly pipeline.
 */

const PLATFORMS = ['netflix', 'youtube', 'disneyplus', 'amazonprime', 'crunchyroll', 'viki'] as const;
const FIXTURES_DIR = path.resolve(__dirname, '..', 'fixtures', 'streaming');
const EXTENSION_DIR = path.resolve(__dirname, '..', '..', 'apps', 'extension', 'dist', 'chrome');
const PLATFORM_URLS: Record<(typeof PLATFORMS)[number], string> = {
  netflix: 'https://www.netflix.com/watch/81234567',
  youtube: 'https://www.youtube.com/watch?v=carvetest',
  disneyplus: 'https://www.disneyplus.com/video/carvetest',
  amazonprime: 'https://www.primevideo.com/video/detail/carvetest',
  crunchyroll: 'https://www.crunchyroll.com/watch/carvetest',
  viki: 'https://www.viki.com/videos/123456v-carve-test',
};

test.describe('extension — streaming-platform fixtures', () => {
  test.skip(!fs.existsSync(EXTENSION_DIR), 'extension dist not built — run pnpm --filter @carve/extension build first');
  test.describe.configure({ mode: 'serial' });

  for (const platform of PLATFORMS) {
    test(`${platform} hook fires on recorded DOM`, async ({ browserName }, testInfo) => {
      test.skip(browserName !== 'chromium', 'Chrome extension fixtures run once under Chromium');

      const fixturePath = path.join(FIXTURES_DIR, `${platform}.html`);
      if (!fs.existsSync(fixturePath)) {
        test.skip(true, `fixture for ${platform} not yet recorded`);
        return;
      }
      const fixture = fs.readFileSync(fixturePath, 'utf8');
      const fixtureUrl = PLATFORM_URLS[platform];

      const context = await chromium.launchPersistentContext(testInfo.outputPath(`profile-${platform}`), {
        headless: false,
        args: [
          `--disable-extensions-except=${EXTENSION_DIR}`,
          `--load-extension=${EXTENSION_DIR}`,
        ],
      });

      try {
        let [sw] = context.serviceWorkers();
        if (!sw) sw = await context.waitForEvent('serviceworker');
        await sw.evaluate(async () => {
          await chrome.storage.local.set({ targetLanguage: 'en', annotateLatinSites: true });
        });

        const page = await context.newPage();
        await page.route('**/*', (route) => {
          if (route.request().url() === fixtureUrl) {
            return route.fulfill({ status: 200, contentType: 'text/html', body: fixture });
          }
          return route.abort();
        });
        await page.goto(fixtureUrl);
        // Inject the carve overlay's hook trigger by simulating the DOM
        // mutations that the recorded fixture asserts cause `onCue` to fire.
        await page.waitForFunction(
          () => !!document.querySelector('#carve-sub-overlay'),
          { timeout: 5_000 },
        ).catch(async () => {
          const found = await page.evaluate(() => Array.from(document.querySelectorAll('*'))
            .slice(0, 50).map((el) => el.tagName + (el.id ? `#${el.id}` : '')).join(', '));
          throw new Error(`overlay never mounted on ${platform} fixture; first selectors: ${found}`);
        });
        await expect(page.locator('#cso-target')).not.toContainText('Subtitles will appear here');
      } finally {
        await context.close();
      }
    });
  }

  test('youtube captions render in chunks instead of word-by-word', async ({ browserName }, testInfo) => {
    test.skip(browserName !== 'chromium', 'Chrome extension fixtures run once under Chromium');

    const fixturePath = path.join(FIXTURES_DIR, 'youtube.html');
    const fixture = fs.readFileSync(fixturePath, 'utf8').replace('Active YouTube caption text.', '');
    const fixtureUrl = PLATFORM_URLS.youtube;

    const context = await chromium.launchPersistentContext(testInfo.outputPath('profile-youtube-progressive'), {
      headless: false,
      args: [
        `--disable-extensions-except=${EXTENSION_DIR}`,
        `--load-extension=${EXTENSION_DIR}`,
      ],
    });

    try {
      let [sw] = context.serviceWorkers();
      if (!sw) sw = await context.waitForEvent('serviceworker');
      await sw.evaluate(async () => {
        await chrome.storage.local.set({ targetLanguage: 'en', annotateLatinSites: true });
      });

      const page = await context.newPage();
      await page.route('**/*', (route) => {
        if (route.request().url() === fixtureUrl) {
          return route.fulfill({ status: 200, contentType: 'text/html', body: fixture });
        }
        return route.abort();
      });
      await page.goto(fixtureUrl);
      await page.waitForFunction(() => !!document.querySelector('#carve-sub-overlay'), { timeout: 5_000 });

      const setCaption = async (text: string) => {
        await page.evaluate((next) => {
          const segment = document.querySelector<HTMLElement>('.ytp-caption-segment');
          if (!segment) throw new Error('missing YouTube caption segment');
          segment.textContent = next;
        }, text);
        await page.waitForTimeout(80);
      };

      const target = page.locator('#cso-target');
      await setCaption('I');
      await expect(target).toHaveText('');

      await setCaption('I had');
      await expect(target).toHaveText('');

      await setCaption('I had a');
      await expect(target).toHaveText('');

      await setCaption('I had a conversation');
      await expect(target).toHaveText('I had a conversation');

      await setCaption('I had a conversation recently');
      await expect(target).toHaveText('I had a conversation');
      await page.waitForTimeout(450);
      await expect(target).toHaveText('I had a conversation');

      await setCaption('I had a conversation recently that I have');
      await expect(target).toHaveText('I had a conversation recently that I have');
    } finally {
      await context.close();
    }
  });
});
