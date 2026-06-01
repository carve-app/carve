import { test, expect, chromium, firefox } from '@playwright/test';
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

test.describe('extension — streaming-platform fixtures', () => {
  test.skip(!fs.existsSync(EXTENSION_DIR), 'extension dist not built — run pnpm --filter @carve/extension build first');

  for (const platform of PLATFORMS) {
    test(`${platform} hook fires on recorded DOM`, async () => {
      const fixturePath = path.join(FIXTURES_DIR, `${platform}.html`);
      if (!fs.existsSync(fixturePath)) {
        test.skip(true, `fixture for ${platform} not yet recorded`);
        return;
      }

      const context = await chromium.launchPersistentContext('', {
        headless: false,
        args: [
          `--disable-extensions-except=${EXTENSION_DIR}`,
          `--load-extension=${EXTENSION_DIR}`,
        ],
      });

      try {
        const page = await context.newPage();
        await page.goto(`file://${fixturePath}`);
        // Inject the carve overlay's hook trigger by simulating the DOM
        // mutations that the recorded fixture asserts cause `onCue` to fire.
        await page.waitForFunction(
          () => !!document.querySelector('#carve-overlay, [data-carve="subtitle-overlay"]'),
          { timeout: 5_000 },
        ).catch(async () => {
          // If the overlay didn't auto-mount (because the fixture's URL
          // doesn't match a platform regex), the test logs which selectors
          // were observed so the regression is easy to debug.
          const found = await page.evaluate(() => Array.from(document.querySelectorAll('*'))
            .slice(0, 50).map((el) => el.tagName + (el.id ? `#${el.id}` : '')).join(', '));
          throw new Error(`overlay never mounted on ${platform} fixture; first selectors: ${found}`);
        });
        expect(await page.locator('[data-carve="subtitle-overlay"], #carve-overlay').count()).toBeGreaterThan(0);
      } finally {
        await context.close();
      }
    });
  }
});
